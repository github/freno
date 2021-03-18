package config

import (
	"testing"

	test "github.com/outbrain/golib/tests"
)

func TestProxySQLAddressToDSN(t *testing.T) {
	{
		c := &ProxySQLConfigurationSettings{User: "freno"}
		test.S(t).ExpectEquals(c.AddressToDSN("proxysql-123abcd.test:6032"), "mysql://freno:*****@proxysql-123abcd.test:6032/"+ProxySQLDefaultDatabase)
	}
}

func TestProxySQLIsEmpty(t *testing.T) {
	{
		c := &ProxySQLConfigurationSettings{}
		isEmpty := c.IsEmpty()
		test.S(t).ExpectTrue(isEmpty)
	}
	{
		c := &ProxySQLConfigurationSettings{Addresses: []string{"localhost:6032"}}
		isEmpty := c.IsEmpty()
		test.S(t).ExpectTrue(isEmpty)
	}
	{
		c := &ProxySQLConfigurationSettings{Addresses: []string{"localhost:6032"}, User: "freno"}
		isEmpty := c.IsEmpty()
		test.S(t).ExpectTrue(isEmpty)
	}
	{
		c := &ProxySQLConfigurationSettings{Addresses: []string{"localhost:6032"}, User: "freno", Password: "freno"}
		isEmpty := c.IsEmpty()
		test.S(t).ExpectTrue(isEmpty)
	}
	{
		c := &ProxySQLConfigurationSettings{Addresses: []string{"localhost:6032"}, User: "freno", Password: "freno", HostgroupID: 20}
		isEmpty := c.IsEmpty()
		test.S(t).ExpectFalse(isEmpty)
	}
}
