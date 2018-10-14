package config

//
// Meta-checks configuration
// A meta check aggregates one or more other checks, and will return the worst.
//

type MetaChecksListing []string

type MetaChecks map[string]MetaChecksListing
