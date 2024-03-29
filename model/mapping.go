package model

import (
	"strings"
	"unicode"
)

// Updates the default struct to column name mapper (you can still bypass this using the `db:"col_name"` tag)
// sample:
//   // struct:
//   type Company struct { ID int, ContactPerson int }
//
//   // query:
//   select id, contactPerson from company where id = 1;
//
//   // mapper:
//   mapper("ID") // id
//   mapper("ContactPerson") // contactPerson
//
func SetStructNameMapping(mapper func(structCol string) (colName string)) {
	muAll.RLock()
	defer muAll.RUnlock()

	defaultDb.MapperFunc(mapper)

	for _, db := range otherDbs {
		db.MapperFunc(mapper)
	}
}

func SnakeCaseStructNameMapping(structCol string) string {

	if len(structCol) == 0 {
		return structCol
	}

	src := []rune(structCol)
	var dst []rune

	for i, c := range src {
		if i == 0 {
			dst = append(dst, unicode.ToLower(c))
			continue
		}

		if unicode.IsUpper(c) {
			if unicode.IsLower(src[i-1]) || unicode.IsDigit(src[i-1]) {
				dst = append(dst, '_')
			}

			dst = append(dst, unicode.ToLower(c))
			continue
		}

		if unicode.IsDigit(c) {
			if !unicode.IsDigit(src[i-1]) {
				dst = append(dst, '_')
			}

			dst = append(dst, c)
			continue
		}

		dst = append(dst, c)
	}

	return string(dst)
}

func LowerCamelCaseStructNameMapping(structCol string) string {
	if len(structCol) == 0 {
		return structCol
	}

	if structCol == "ID" {
		return "id"
	}

	if structCol == "URL" {
		return "url"
	}

	structCol = strings.Replace(structCol, "ID", "Id", -1)
	structCol = strings.Replace(structCol, "URL", "Url", -1)

	runes := []rune(structCol)
	runes[0] = unicode.ToLower(runes[0])

	return string(runes)
}
