/*
   Copyright 2017 GitHub Inc.
	 See https://github.com/github/freno/blob/master/LICENSE
*/

package mysql

import (
	"testing"

	"github.com/outbrain/golib/log"
	test "github.com/outbrain/golib/tests"
)

func init() {
	log.SetLevel(log.ERROR)
}

func TestNewRawInstanceKey(t *testing.T) {
	{
		key, err := newRawInstanceKey("127.0.0.1:3307")
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(key.Hostname, "127.0.0.1")
		test.S(t).ExpectEquals(key.Port, 3307)
	}
	{
		_, err := newRawInstanceKey("127.0.0.1:abcd")
		test.S(t).ExpectNotNil(err)
	}
	{
		_, err := newRawInstanceKey("127.0.0.1:")
		test.S(t).ExpectNotNil(err)
	}
	{
		_, err := newRawInstanceKey("127.0.0.1")
		test.S(t).ExpectNotNil(err)
	}
}

func TestParseInstanceKey(t *testing.T) {
	{
		key, err := ParseInstanceKey("127.0.0.1:3307", 3306)
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(key.Hostname, "127.0.0.1")
		test.S(t).ExpectEquals(key.Port, 3307)
	}
	{
		key, err := ParseInstanceKey("127.0.0.1", 3306)
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(key.Hostname, "127.0.0.1")
		test.S(t).ExpectEquals(key.Port, 3306)
	}
}

func TestEquals(t *testing.T) {
	{
		expect := &InstanceKey{Hostname: "127.0.0.1", Port: 3306}
		key, err := ParseInstanceKey("127.0.0.1", 3306)
		test.S(t).ExpectNil(err)
		test.S(t).ExpectTrue(key.Equals(expect))
	}
}

func TestStringCode(t *testing.T) {
	{
		key := &InstanceKey{Hostname: "127.0.0.1", Port: 3306}
		stringCode := key.StringCode()
		test.S(t).ExpectEquals(stringCode, "127.0.0.1:3306")
	}
}
