// Package lib is the healthy declaring package.
package lib

// Hello is called from app and from the broken brokenuser.
func Hello(name string) string {
	return "hello " + name
}

// Speaker is the interface impls is asked about.
type Speaker interface {
	Speak() string
}

// Config is the struct tags is asked about.
type Config struct {
	Name string `json:"name"`
}

// Unused is called by nobody, so dead list always has something to report.
func Unused() int {
	return 0
}
