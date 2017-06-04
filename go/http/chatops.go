//
// Chatops-RPC
// This is intended to provide well formed API information to hubot.
// GitHub's implementation is its own, but this can serve other automation as well.
//

package http

import (
	"fmt"
)

const frenoNamespace = "freno"

var EmptyParams = []string{}

type ChatopsMethod struct {
	Help   string   `json:"help"`
	Regex  string   `json:"regex"`
	Params []string `json:"params"`
	Path   string   `json:"path"`
}

func NewChatopsMethod(regexSuffix string, params []string, path string, help string) *ChatopsMethod {
	return &ChatopsMethod{
		Regex:  fmt.Sprintf(`^%s\s+%s`, frenoNamespace, regexSuffix),
		Params: params,
		Path:   path,
		Help:   help,
	}
}

type Chatops struct {
	Namespace     string
	Help          string
	Version       int
	ErrorResponse string
	Methods       map[string]ChatopsMethod
}

func NewChatops() *Chatops {
	return &Chatops{
		Namespace:     frenoNamespace,
		Help:          "n/a",
		Version:       2,
		ErrorResponse: "see freno documentation, https://github.com/github/freno/tree/master/doc",
		Methods:       make(map[string]ChatopsMethod),
	}
}

func (chatops *Chatops) AddMethod(name string, method *ChatopsMethod) {
	chatops.Methods[name] = *method
}
