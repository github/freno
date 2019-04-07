/*
   Copyright 2019 GitHub Inc.
	 See https://github.com/github/freno/blob/master/LICENSE
*/

package config

import (
	"testing"

	"github.com/outbrain/golib/log"
	test "github.com/outbrain/golib/tests"
)

func init() {
	log.SetLevel(log.ERROR)
}

func TestParseAddresses(t *testing.T) {
	{
		c := &HAProxyConfigurationSettings{Addresses: ""}
		addresses, err := c.parseAddresses()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(len(addresses), 0)
	}
	{
		c := &HAProxyConfigurationSettings{Addresses: ",,, , , , ,,"}
		addresses, err := c.parseAddresses()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(len(addresses), 0)
	}
	{
		c := &HAProxyConfigurationSettings{Addresses: ",,, , , , ,localhost:1234 ,"}
		addresses, err := c.parseAddresses()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(len(addresses), 1)
		test.S(t).ExpectEquals(addresses[0].String(), "localhost:1234")
	}
	{
		c := &HAProxyConfigurationSettings{Addresses: "localhost:1234,otherhost:5678"}
		addresses, err := c.parseAddresses()
		test.S(t).ExpectNil(err)
		test.S(t).ExpectEquals(len(addresses), 2)
		test.S(t).ExpectEquals(addresses[0].String(), "localhost:1234")
		test.S(t).ExpectEquals(addresses[1].String(), "otherhost:5678")
	}
	{
		c := &HAProxyConfigurationSettings{Addresses: "localhost"}
		_, err := c.parseAddresses()
		test.S(t).ExpectNotNil(err)
	}
	{
		c := &HAProxyConfigurationSettings{Addresses: "localhost:"}
		_, err := c.parseAddresses()
		test.S(t).ExpectNotNil(err)
	}
	{
		c := &HAProxyConfigurationSettings{Addresses: "localhost:abcd"}
		_, err := c.parseAddresses()
		test.S(t).ExpectNotNil(err)
	}
	{
		c := &HAProxyConfigurationSettings{Addresses: "localhost:abcd:1234"}
		_, err := c.parseAddresses()
		test.S(t).ExpectNotNil(err)
	}
}

func TestGetProxyAddresses(t *testing.T) {
	{
		c := &HAProxyConfigurationSettings{Addresses: ""}
		addresses := c.GetProxyAddresses()
		test.S(t).ExpectEquals(len(addresses), 0)
	}
	{
		c := &HAProxyConfigurationSettings{Addresses: ",,, , , , ,,"}
		addresses := c.GetProxyAddresses()
		test.S(t).ExpectEquals(len(addresses), 0)
	}
	{
		c := &HAProxyConfigurationSettings{Addresses: ",,, , , , ,localhost:1234 ,"}
		addresses := c.GetProxyAddresses()
		test.S(t).ExpectEquals(len(addresses), 1)
		test.S(t).ExpectEquals(addresses[0].String(), "localhost:1234")
	}
	{
		c := &HAProxyConfigurationSettings{Addresses: "localhost:1234,otherhost:5678"}
		addresses := c.GetProxyAddresses()
		test.S(t).ExpectEquals(len(addresses), 2)
		test.S(t).ExpectEquals(addresses[0].String(), "localhost:1234")
		test.S(t).ExpectEquals(addresses[1].String(), "otherhost:5678")
	}
	{
		c := &HAProxyConfigurationSettings{Host: "explicit", Port: 1000, Addresses: "localhost:1234,otherhost:5678"}
		addresses := c.GetProxyAddresses()
		test.S(t).ExpectEquals(len(addresses), 1)
		test.S(t).ExpectEquals(addresses[0].String(), "explicit:1000")
	}
}

func TestIsEmpty(t *testing.T) {
	{
		c := &HAProxyConfigurationSettings{}
		isEmpty := c.IsEmpty()
		test.S(t).ExpectTrue(isEmpty)
	}
	{
		c := &HAProxyConfigurationSettings{Host: "localhost"}
		isEmpty := c.IsEmpty()
		test.S(t).ExpectTrue(isEmpty)
	}
	{
		c := &HAProxyConfigurationSettings{Host: "localhost", Port: 1234}
		isEmpty := c.IsEmpty()
		test.S(t).ExpectTrue(isEmpty)
	}
	{
		c := &HAProxyConfigurationSettings{Host: "localhost", Port: 1234, PoolName: "p_ro"}
		isEmpty := c.IsEmpty()
		test.S(t).ExpectFalse(isEmpty)
	}
	{
		c := &HAProxyConfigurationSettings{Addresses: "localhost:1234,otherhost:5678", PoolName: "p_ro"}
		isEmpty := c.IsEmpty()
		test.S(t).ExpectFalse(isEmpty)
	}
}
