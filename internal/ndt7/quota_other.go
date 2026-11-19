//go:build !windows

package ndt7

func isTransient(error) bool { return false }
