package services

import "testing"

func TestParseSkillFrontmatter(t *testing.T) {
	content := `---
name: inbox-router
description: Route structured inbox messages.
when_to_use: When triaging agent inbox work.
allowed_tools:
  - read_inbox
  - send_message
tags:
  - inbox
  - routing
activation:
  path_patterns:
    - /inbox/**
---
# Inbox Router

Routes messages into the right workflow.`

	meta, description, err := parseSkillFrontmatter(content)
	if err != nil {
		t.Fatalf("parseSkillFrontmatter() error = %v", err)
	}
	if got := meta["name"]; got != "inbox-router" {
		t.Fatalf("name = %v", got)
	}
	if got := meta["when_to_use"]; got != "When triaging agent inbox work." {
		t.Fatalf("when_to_use = %v", got)
	}
	if description != "Routes messages into the right workflow." {
		t.Fatalf("description = %q", description)
	}
}

func TestClassifyEntryKind(t *testing.T) {
	cases := map[string]string{
		"/memory/profile/preferences.md":     "memory_profile",
		"/memory/scratch/2026-04-04/note.md": "memory_scratch",
		"/projects/demo/context.md":          "project_context",
		"/projects/demo/log.jsonl":           "project_log",
		"/inbox/assistant/incoming/id.json":  "inbox_message",
		"/devices/light/SKILL.md":            "device_skill",
		"/roles/researcher/SKILL.md":         "role_skill",
		"/skills/write/SKILL.md":             "skill",
	}

	for path, want := range cases {
		if got := classifyEntryKind(path, false); got != want {
			t.Fatalf("classifyEntryKind(%q) = %q, want %q", path, got, want)
		}
	}
	if got := classifyEntryKind("/projects/demo/", true); got != "directory" {
		t.Fatalf("directory kind = %q", got)
	}
}
