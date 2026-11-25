// Package app is a hand written user of store.
package app

import "example.com/fixture/store"

// Run calls through the concrete type, through the interface and through embedding.
func Run(q store.Querier, buffer []byte) {
	h := store.Open()   // ref:app-open
	_ = h.Exec("a")     // ref:app-exec
	_ = h.Query("b")    // ref:app-query
	_ = h.QueryRow("c") // ref:app-queryrow
	_ = h.Close()       // ref:app-close
	exec := h.Exec      // ref:app-exec-value
	_ = exec
	_ = q.Query("d")      // ref:app-iface-query
	_, _ = q.Read(buffer) // ref:app-iface-read
	box := store.Box[int]{}
	_ = box.Get() // ref:app-box-get
	_ = store.Plain{N: store.Limit}
	_ = store.Default
}
