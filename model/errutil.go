package model

import "github.com/lib/pq"

func IsDuplicateKeyError(err error) bool {
	pErr, ok := err.(*pq.Error)
	if !ok {
		return false
	}

	return pErr.Code == "23505"
}
