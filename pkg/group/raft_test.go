package group

import (
	"testing"

	"github.com/github/freno/pkg/config"

	test "github.com/outbrain/golib/tests"
)

func TestNormalizeRaftNode(t *testing.T) {
	config.Settings().DefaultRaftPort = 10008
	{
		node := ":1234"
		normalizedNode := normalizeRaftNode(node)
		test.S(t).ExpectEquals(normalizedNode, node)
	}
	{
		node := "localhost:1234"
		normalizedNode := normalizeRaftNode(node)
		test.S(t).ExpectEquals(normalizedNode, node)
	}
	{
		node := "localhost"
		normalizedNode := normalizeRaftNode(node)
		test.S(t).ExpectEquals(normalizedNode, "localhost:10008")
	}
	{
		node := ""
		normalizedNode := normalizeRaftNode(node)
		test.S(t).ExpectEquals(normalizedNode, ":10008")
	}

	config.Settings().DefaultRaftPort = 0
	{
		node := "localhost"
		normalizedNode := normalizeRaftNode(node)
		test.S(t).ExpectEquals(normalizedNode, node)
	}
}
