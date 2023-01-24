/*
   Copyright 2023 GitHub Inc.
	 See https://github.com/github/freno/blob/master/LICENSE
*/

package group

import (
	"testing"

	"github.com/github/freno/pkg/config"
	"github.com/outbrain/golib/log"
	test "github.com/outbrain/golib/tests"
)

func init() {
	log.SetLevel(log.ERROR)
}

func TestGetBackendDBUri(t *testing.T) {
	config.Settings().BackendMySQLUser = "gromit"
	config.Settings().BackendMySQLPassword = "penguin"
	config.Settings().BackendMySQLHost = "myhost"
	config.Settings().BackendMySQLPort = 3306
	config.Settings().BackendMySQLSchema = "test_database"

	// test default (charset)
	dbUri := GetBackendDBUri()
	test.S(t).ExpectEquals(dbUri, "gromit:penguin@tcp(myhost:3306)/test_database?interpolateParams=true&charset=utf8mb4,utf8,latin1&timeout=500ms")

	// test setting collation
	config.Settings().BackendMySQLCollation = "utf8mb4_unicode_ci"
	dbUri = GetBackendDBUri()
	test.S(t).ExpectEquals(dbUri, "gromit:penguin@tcp(myhost:3306)/test_database?interpolateParams=true&collation=utf8mb4_unicode_ci&timeout=500ms")
}
