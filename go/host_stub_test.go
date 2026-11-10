//go:build !cgo

package main

import "fmt"

func hostRPC(string, any, any) error {
	return fmt.Errorf("host RPC unavailable in this test build")
}
