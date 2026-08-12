package store

import (
	_ "embed"
)

const schemaVersion = 4

//go:embed schema.sql
var schema string

func Schema() string {
	return schema
}

func Version() int {
	return schemaVersion
}
