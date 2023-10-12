package rqutil

import (
	"context"
	"fmt"
	"github.com/Masterminds/squirrel"
	"github.com/lann/builder"
	"github.com/ntbosscher/gobase/auth"
	"github.com/ntbosscher/gobase/er"
	"github.com/ntbosscher/gobase/model"
	"github.com/ntbosscher/gobase/model/squtil"
	"github.com/ntbosscher/gobase/res"
	"reflect"
	"strings"
	"time"
)

var mapper = model.SnakeCaseStructNameMapping

func MustParse[T any](r *res.Request, value T) T {
	r.MustParseJSON(value)
	return value
}

func UpsertModelAllFields(ctx context.Context, obj interface{}, table string) int {
	return UpsertModelExcept(ctx, obj, table)
}

func splitToMap(input string, splitOn string) map[string]bool {
	out := map[string]bool{}
	for _, item := range strings.Split(input, splitOn) {
		out[item] = true
	}

	return out
}

func UpsertModelExcept(ctx context.Context, obj interface{}, table string, ignore ...string) int {
	el := reflect.TypeOf(obj)
	val := reflect.ValueOf(obj)

	if el.Kind() == reflect.Ptr {
		el = el.Elem()
		val = val.Elem()
	}

	fields := []string{}

	defaultIgnoredFields := map[string]bool{
		"UpdatedAt": true,
		"CreatedAt": true,
		"IDs":       true,
	}

	for _, field := range ignore {
		defaultIgnoredFields[field] = true
	}

	for i := 0; i < el.NumField(); i++ {
		fld := el.Field(i)
		tag := splitToMap(fld.Tag.Get("rq"), ",")

		if tag["upsert_ignore"] {
			continue
		}

		if tag["omitempty"] {
			if val.Field(i).IsZero() {
				continue
			}
		}

		if defaultIgnoredFields[fld.Name] {
			continue
		}

		fields = append(fields, fld.Name)
	}

	return UpsertModel(ctx, obj, table, fields...)
}

type Config struct {
	NoSortOrderUpdate bool
}

type configKeyType string

const configKey configKeyType = "config"

func WithConfig(ctx context.Context, config *Config) context.Context {
	return context.WithValue(ctx, configKey, config)
}

func getConfig(ctx context.Context) *Config {
	v := ctx.Value(configKey)
	if v == nil {
		return &Config{}
	}

	return v.(*Config)
}

func UpsertModel(ctx context.Context, obj interface{}, table string, fields ...string) int {
	el := reflect.ValueOf(obj)
	if el.Kind() == reflect.Ptr {
		el = el.Elem()
	}

	typ := el.Type()

	noUpdateFields := []string{}
	mp := map[string]interface{}{
		"updated_at": time.Now().UTC(),
	}

	ignore := map[string]bool{
		"created_at": true,
		"id":         true,
	}

	for _, field := range fields {
		fld := el.FieldByName(field)

		if !fld.IsValid() {
			er.Throw(fmt.Sprintf("field '%s' isn't valid on type %s", field, el.String()))
		}

		fTyp, _ := typ.FieldByName(field)
		dbName := fTyp.Tag.Get("db")
		name := ""

		if dbName == "-" {
			continue
		} else if dbName != "" {
			name = dbName
		} else {
			name = mapper(field)
		}

		if ignore[name] {
			continue
		}

		rq := splitToMap(fTyp.Tag.Get("rq"), ",")
		if rq["no_update"] {
			noUpdateFields = append(noUpdateFields, name)
		}

		if rq["-"] {
			continue
		}

		mp[name] = fld.Interface()
	}

	idField := el.FieldByName("ID")
	if !idField.IsValid() {
		er.Throw(fmt.Sprintf("field ID is required on type %s", el.String()))
	}

	hasUpdatedBy := el.FieldByName("UpdatedBy").IsValid()
	if hasUpdatedBy {
		usr := auth.UserNull(ctx)

		if usr.Valid {
			mp["updated_by"] = usr.Int
		}
	}

	id := idField.Int()

	if id <= 0 {
		mp["created_at"] = time.Now().UTC()
		id = squtil.MustInsert(ctx, model.Builder.Insert(table).SetMap(mp).
			Suffix("returning id"))
	} else {
		for _, field := range noUpdateFields {
			delete(mp, field)
		}

		squtil.MustExecContext(ctx, model.Builder.Update(table).SetMap(mp).Where(squirrel.Eq{"id": id}))
	}

	handleSortOrder(ctx, el, table, id)
	return int(id)
}

func handleSortOrder(ctx context.Context, el reflect.Value, table string, id int64) {
	if getConfig(ctx).NoSortOrderUpdate {
		return
	}

	typ := el.Type()

	orderCol := ""
	partition := ""

	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		tag := splitToMap(f.Tag.Get("rq"), ",")
		if tag["sort_order_partition"] {
			partition = "partition by " + mapper(f.Name)
		}

		if tag["sort_order"] {
			if orderCol != "" {
				er.Throw("can only have 1 `rq:sort_order tag per struct`")
			}

			orderCol = mapper(f.Name)
		}
	}

	if orderCol == "" {
		return
	}

	model.MustExecContext(ctx, `
		update `+table+` t1
		set 
		    `+orderCol+` = t2.rank,
		    updated_at = current_timestamp
		from (
		    select id, rank() over (
		        `+partition+` 
		        order by `+orderCol+`, 
		            case when id = $1 then 0 else id end -- current id first, other ids in insert-order
			) "rank"
		    from `+table+`
		) t2
		where t2.id = t1.id and t2.rank != t1.`+orderCol+` 
	`, id)
}

func ID(value int) res.Responder {
	return res.Ok(map[string]interface{}{
		"id": value,
	})
}

func Get[T any](rq *res.Request, table string) res.Responder {
	value := GetRaw[T](rq.Context(), table, rq.GetQueryInt("id"))
	return res.Ok(value)
}

func GetRaw[T any](ctx context.Context, table string, id int) T {

	var item T
	rootTyp := reflect.TypeOf(item)

	if rootTyp.Kind() != reflect.Ptr {
		er.Throw("T must be a pointer type (e.g. *Project)")
	}

	typ := rootTyp.Elem()
	item = reflect.New(typ).Interface().(T)

	columns := []string{}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		if field.Tag.Get("rq") == "-" {
			continue
		}

		columns = append(columns, mapper(field.Name))
	}

	selector := model.Builder.Select(columns...).From(table).Where(squirrel.Eq{"id": id})
	squtil.MustGetContext(ctx, item, selector)

	return item
}

func GetListQr[T any](rq *res.Request, qr squirrel.SelectBuilder, searchFields []string) res.Responder {
	return GetListQrOpt(rq, qr, &GetListOpts[T]{
		SearchFields: searchFields,
	})
}

type GetListOpts[T any] struct {
	SearchFields []string
	ProcessItems func(input T)
}

func GetListQrOpt[T any](rq *res.Request, qr squirrel.SelectBuilder, params *GetListOpts[T]) res.Responder {
	list := []T{}

	offset := rq.GetQueryInt("offset")
	search := rq.Query("search")
	archived := rq.Query("archived")

	if search != "" {
		or := squirrel.Or{}

		for _, field := range params.SearchFields {
			like := squirrel.ILike{}
			like[mapper(field)] = "%" + search + "%"
			or = append(or, like)
		}

		qr = qr.Where(or)
	}

	qr = qr.Where(squirrel.Eq{"archived": archived == "true"})

	value, ok := builder.Get(qr, "From")
	if ok {
		str, _, _ := value.(squirrel.Sqlizer).ToSql()
		parts := strings.Split(str, " ")
		last := parts[len(parts)-1]
		qr = qr.OrderBy(last + ".id desc")
	} else {
		qr = qr.OrderBy("id desc")
	}

	squtil.MustSelectContext(rq.Context(), &list, qr.Offset(uint64(offset)).Limit(100))

	if params.ProcessItems != nil {
		for _, item := range list {
			params.ProcessItems(item)
		}
	}

	count := 0
	squtil.MustGetContext(rq.Context(), &count, model.Builder.Select("count(*)").FromSelect(qr, "d"))

	return res.Ok(map[string]interface{}{
		"data":  list,
		"count": count,
	})
}

func GetList[T any](rq *res.Request, table string, searchFields []string) res.Responder {

	var item T
	typ := reflect.TypeOf(item).Elem()

	columns := []string{}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		if field.Tag.Get("rq") == "-" {
			continue
		}

		dbTag := field.Tag.Get("db")
		switch dbTag {
		case "-":
			continue
		case "":
			columns = append(columns, mapper(field.Name))
		default:
			columns = append(columns, dbTag)
		}
	}

	selector := model.Builder.Select(columns...).From(table)
	return GetListQr[T](rq, selector, searchFields)
}

func DebugSQL(qr squirrel.SelectBuilder) {
	sql, params, _ := qr.ToSql()

	for i, item := range params {
		dollar := fmt.Sprintf("$%d", i+1)

		switch item.(type) {
		case string:
			sql = strings.ReplaceAll(sql, dollar, fmt.Sprintf("'%s'", item))
		case bool:
			sql = strings.ReplaceAll(sql, dollar, fmt.Sprintf("%v", item))
		case int:
			sql = strings.ReplaceAll(sql, dollar, fmt.Sprintf("%d", item))
		case int64:
			sql = strings.ReplaceAll(sql, dollar, fmt.Sprintf("%d", item))
		default:
			er.Throw("unknown type")
		}
	}

	fmt.Println(sql)
}
