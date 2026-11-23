// Package lib is the healthy declaring package every lookup targets.
package lib

// Hello is the exported function whose references are looked up.
func Hello(name string) string {
	return "hello " + name
}

// Speaker is the interface whose implementations are looked up.
type Speaker interface {
	Speak() string
}

// Config is the struct whose tags are looked up.
type Config struct {
	Name string `json:"name"`
	Port int    `json:"port,omitempty"`
}

// LibSpeaker implements Speaker in the healthy declaring package.
type LibSpeaker struct{}

// Speak makes LibSpeaker a Speaker.
func (LibSpeaker) Speak() string {
	return "lib"
}
