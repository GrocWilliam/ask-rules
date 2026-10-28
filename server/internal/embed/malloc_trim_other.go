//go:build !linux || !cgo

package embed

// trimNativeHeap : malloc_trim n'existe qu'avec glibc (voir malloc_trim_linux.go).
func trimNativeHeap() {}
