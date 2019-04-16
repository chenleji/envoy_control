package nodehash

import (
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
)

// NodeHash returns node ID as an ID
type NodeHash struct {
}

// ID function
func (h NodeHash) ID(node *core.Node) string {
	if node == nil {
		return "unknown"
	}
	return node.Id
}
