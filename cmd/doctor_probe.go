package cmd

import (
	"fmt"
)

// DoctorProbeCmd is an internal hidden command used by `notebrain doctor` to
// test whether the ChromaDB store can be opened. A corrupted native HNSW
// index aborts the process with SIGABRT; doctor interprets that as database
// corruption, so this must run in its own process.
type DoctorProbeCmd struct{}

// Run opens the store and forces both HNSW indexes to load. Success is
// reported via exit code 0; any Go-level failure is returned as an error and
// a native abort kills the process (parent sees the signal).
func (c *DoctorProbeCmd) Run(globals *Globals) error {
	st, err := openStore(globals.Ctx, globals)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = st.Close() }()

	fmt.Println("probe ok")
	return nil
}
