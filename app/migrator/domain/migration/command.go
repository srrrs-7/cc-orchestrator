package migration

import "fmt"

// validCommands is the closed set of Command values this domain
// forwards to a Runner (SPEC-005 R5: "-command up|down|status").
var validCommands = map[string]bool{"up": true, "down": true, "status": true}

// Command is the goose operation a migrator run applies: up, down, or
// status. Like Target, it is a value object only ever produced via
// ParseCommand.
type Command struct {
	value string
}

// ParseCommand validates s against the closed set of recognized goose
// commands, rejecting anything else.
func ParseCommand(s string) (Command, error) {
	if !validCommands[s] {
		return Command{}, fmt.Errorf("migrator: command must be one of up, down, status (got %q)", s)
	}
	return Command{value: s}, nil
}

// String returns the underlying command string ("up", "down", or
// "status").
func (c Command) String() string {
	return c.value
}

// IsUp reports whether c is the "up" command. service.Service uses
// this to decide whether a requested least-privilege role provisioning
// step (ISSUE-016 R-c) runs: "down" and "status" leave any requested
// role untouched, since GRANTing access to tables a "down" run may be
// about to drop (or a "status" run never touches) makes no sense.
func (c Command) IsUp() bool {
	return c.value == "up"
}
