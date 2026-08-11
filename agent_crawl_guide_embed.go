package main

import _ "embed"

// Keep the Agent crawl guide inside the single binary so the link remains
// usable in Docker builds and in distributed executable packages.
//
//go:embed doc/SEALCHAT_AGENT_CRAWL_GUIDE_COMPACT.md
var embeddedAgentCrawlGuide string
