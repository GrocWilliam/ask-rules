//go:build linux && cgo

package service

// #include <malloc.h>
import "C"

// trimNativeHeap rend au système la mémoire libérée par ONNX Runtime : glibc
// la garde sinon dans ses arènes et la RSS ne baisse jamais après Release().
func trimNativeHeap() { C.malloc_trim(0) }
