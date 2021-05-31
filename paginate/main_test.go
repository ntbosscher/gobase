package paginate

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/ntbosscher/gobase/er"
	"github.com/ntbosscher/gobase/model"
	"github.com/ntbosscher/gobase/res"
	"net/http"
	"net/url"
	"testing"
)

func ExampleQuery() {
	r := res.NewRouter()
	r.Get("/api/person/list", func(rq *res.Request) res.Responder {

		type Person struct {
			ID         int `csv:"-"` // ignore on csv output
			Email      string
			FullName   string
			Calculated string
		}

		query := model.Builder.Select("id", "email", "concat(first_name, ' ', last_name) as full_name").
			From("person").
			Where(squirrel.Eq{"company": 1})

		cfg := ParamsFromRequest(rq)

		return Query(rq.Context(), &[]*Person{}, query, cfg,
			SearchFields("email", "concat(first_name, ' ', last_name)"),
			ColumnMapping{
				"Name": "full_name",
			},
			ResultProcessor(func(row interface{}) {
				p := row.(*Person)
				p.Calculated = fmt.Sprint(p.ID, "_", p.Email)
			}))
	})

	rq := http.Request{}
	qr := url.Values{
		"search":           []string{"test"},                // apply (LIKE '%test%') filter to all search fields
		"download":         []string{"file.csv"},            // download entire query result as CSV (no paging)
		"filter":           []string{"email=test@test.com"}, // filter specific fields with =,<=,>=,>,< filters
		"orderBy":          []string{"email"},               // sort result before paginating
		"orderByDirection": []string{"asc"},
		"page":             []string{"0"},
		"pageSize":         []string{"50"},
	}
	rq.URL.RawQuery = qr.Encode()
}

func TestQuery(t *testing.T) {
	model.SetStructNameMapping(model.SnakeCaseStructNameMapping)

	type Person struct {
		ID         int `csv:"-"` // ignore on csv output
		Email      string
		FullName   string
		Calculated string
	}

	ctx := context.Background()
	result := &[]*Person{}

	er.Check(model.WithTx(ctx, func(ctx context.Context, tx *sqlx.Tx) error {

		query := model.Builder.Select("id", "concat(first_name, ' ', last_name) as full_name").
			From("person").
			Where(squirrel.Eq{"tenant": 1})

		Query(ctx, result, query,
			SearchFields("concat(first_name, ' ', last_name)"),
			ColumnMapping{
				"Name": "full_name",
			},
			ResultProcessor(func(row interface{}) {
				p := row.(*Person)
				p.Calculated = fmt.Sprint(p.ID, "_", p.FullName)
			}))

		return nil

	}))

	js, _ := json.MarshalIndent(result, "", " ")
	t.Log(string(js))
}
