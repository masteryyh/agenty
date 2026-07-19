package cmd

import "errors"

type exitCodeError struct {
	code int
	err  error
}

func (e *exitCodeError) Error() string {
	return e.err.Error()
}

func (e *exitCodeError) Unwrap() error {
	return e.err
}

func withExitCode(err error, code int) error {
	if err == nil {
		return nil
	}
	return &exitCodeError{code: code, err: err}
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}

	if coded, ok := errors.AsType[*exitCodeError](err); ok {
		return coded.code
	}
	return 1
}
