package ort

import "log"

func logFinalizerWarning(format string, args ...any) {
	// Finalizers may run late during process teardown; guard logging to avoid
	// crashing on best-effort diagnostics.
	defer func() {
		if r := recover(); r != nil {
			_ = r
		}
	}()
	log.Printf(format, args...)
}
