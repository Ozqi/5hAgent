package main

import "testing"

func TestParseAgentTargetsMatchesPSAndAttachCandidates(t *testing.T) {
	output := "0:1.1\t5hagent\tdebug\t/workspace\n0:2.1\ttraex\tcoding\t/repo\n1:3.2\t5hagent\ttask-a\t/project\n"
	targets := parseAgentTargets(output)
	if len(targets) != 2 {
		t.Fatalf("targets = %#v, want two 5hagent panes", targets)
	}
	if targets[0] != (agentTarget{Target: "0:1.1", Work: "debug", CWD: "/workspace"}) {
		t.Fatalf("first target = %#v", targets[0])
	}
	if targets[1].Target != "1:3.2" || targets[1].Work != "task-a" {
		t.Fatalf("second target = %#v", targets[1])
	}
}
