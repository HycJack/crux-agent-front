// Package result implements the Result<T,E> pattern for fallible operations.
// Inspired by pi's Result type: expected failures returned as values, not thrown.
package result

// Result is a sum type: either a success value or an error.
type Result[TValue any, TError any] struct {
	Ok    bool
	Value TValue
	Err   TError
}

// Ok creates a successful Result.
func Ok[TValue any, TError any](value TValue) Result[TValue, TError] {
	return Result[TValue, TError]{Ok: true, Value: value}
}

// Error creates a failed Result.
func Error[TValue any, TError any](err TError) Result[TValue, TError] {
	return Result[TValue, TError]{Ok: false, Err: err}
}

// GetOrThrow returns the value or panics with the error.
func (r Result[TValue, TError]) GetOrThrow() TValue {
	if !r.Ok {
		switch e := any(r.Err).(type) {
		case error:
			panic(e)
		case string:
			panic(e)
		default:
			panic(r.Err)
		}
	}
	return r.Value
}

// GetOrDefault returns the value or the provided default.
func (r Result[TValue, TError]) GetOrDefault(defaultValue TValue) TValue {
	if r.Ok {
		return r.Value
	}
	return defaultValue
}

// Map transforms the success value, leaving errors unchanged.
func Map[TValue any, TError any, U any](r Result[TValue, TError], fn func(TValue) U) Result[U, TError] {
	if r.Ok {
		return Ok[U, TError](fn(r.Value))
	}
	return Error[U, TError](r.Err)
}

// MapError transforms the error value, leaving successes unchanged.
func MapError[TValue any, TError any, E any](r Result[TValue, TError], fn func(TError) E) Result[TValue, E] {
	if r.Ok {
		return Ok[TValue, E](r.Value)
	}
	return Error[TValue, E](fn(r.Err))
}

// FlatMap chains operations that return Results.
func FlatMap[TValue any, TError any, U any](r Result[TValue, TError], fn func(TValue) Result[U, TError]) Result[U, TError] {
	if r.Ok {
		return fn(r.Value)
	}
	return Error[U, TError](r.Err)
}

// Unwrap returns (value, true) on success or (zero, false) on error.
func (r Result[TValue, TError]) Unwrap() (TValue, bool) {
	if r.Ok {
		return r.Value, true
	}
	var zero TValue
	return zero, false
}
