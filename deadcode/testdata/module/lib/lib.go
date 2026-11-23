// Package lib holds functions the analysis must sort into live and dead.
package lib

var initCounter int

// marker:init
func init() {
	initCounter = calledFromInit()
}

// marker:calledFromInit
func calledFromInit() int {
	return 1
}

// marker:Live
func Live() int {
	return initCounter
}

// marker:Abandoned
func Abandoned() int {
	return 2
}

// marker:Soon
func Soon() int {
	return 3
}

// marker:Later
func Later() int {
	return 4
}

// Widget has one method main calls and one nobody calls.
type Widget struct {
	count int
}

// marker:Widget.Used
func (w *Widget) Used() int {
	return w.count
}

// marker:Widget.Unused
func (w *Widget) Unused() int {
	return -w.count
}

// marker:Map
func Map[T any](items []T, change func(T) T) []T {
	changed := make([]T, 0, len(items))
	for _, item := range items {
		changed = append(changed, change(item))
	}
	return changed
}

// marker:NeverInstantiated
func NeverInstantiated[T any](value T) T {
	return value
}

// marker:WithClosure
func WithClosure() int {
	compute := func() int {
		return reachedOnlyThroughClosure()
	}
	return compute()
}

// marker:reachedOnlyThroughClosure
func reachedOnlyThroughClosure() int {
	return 5
}

// marker:AbandonedWithClosure
func AbandonedWithClosure() func() int {
	return func() int {
		return 6
	}
}
