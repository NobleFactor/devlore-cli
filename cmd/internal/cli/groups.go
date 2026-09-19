// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// region Helpers

// ValidateCommandLine reports whether `argv` names a command the tree can run, before anything runs.
//
// The specification's order is parse, validate, run. Cobra's is not: it resolves and dispatches in one pass,
// and validates a command's arguments only after deciding to run it. Two consequences, and this function
// exists for the second:
//
//  1. `Find` checks unknown commands at the ROOT alone (`args.go`, `legacyArgs`): a leaf takes its arguments,
//     the root refuses an unmatched name, and a group -- which has a parent -- accepts it silently.
//  2. `execute` abandons a command that cannot run before it validates: `if !c.Runnable() { return
//     flag.ErrHelp }` precedes `ValidateArgs` by thirteen lines, and `ExecuteC` turns that into help and a nil
//     error.
//
// So `writ repo add team ~/x` printed help and exited 0 with `add` retired, saying nothing about the verb
// having moved (#897). Setting `Args` on groups does not fix it and makes it worse: `Find` consults
// `legacyArgs` only when `Args == nil`, so a validator there disables the root-level check and is itself never
// reached.
//
// [cobra.Command.Find] is exported and free of side effects, so this stands where `legacyArgs` stands -- same
// command, same leftovers, nothing executed -- and refuses what cobra would have accepted. Groups keep
// `Args == nil`, so the root-level check is untouched, and no group gains a `Run` or `RunE`, so §14's
// invariant 7 stands.
//
// Parameters:
//   - `root`: the program's fully built command tree, extensions included.
//   - `argv`: the arguments as the process received them, without the program name.
//
// Returns:
//   - `error`: an [ExitUsage] error naming the verb, or nil when the line is well formed. A resolution failure
//     is nil: cobra reports it, with suggestions, when [cobra.Command.Execute] repeats the walk.
func ValidateCommandLine(root *cobra.Command, argv []string) error {

	command, leftovers, err := root.Find(argv)
	if err != nil || command == nil {
		//nolint:nilerr // a resolution failure is cobra's to report, with its suggestions, when Execute
		// repeats this walk; refusing it here would print it twice and in the wrong voice.
		return nil
	}

	if !command.HasSubCommands() || command.Runnable() {
		return nil
	}

	operands := operandsOf(command, leftovers)
	if len(operands) == 0 {
		return nil
	}

	// Cobra's own validator, run where cobra skipped it -- not a copy of its wording. [cobra.NoArgs] is what
	// `execute` would have reached at line 968 had the group been runnable, so a reader meets the same message
	// under a group as at the root, by construction rather than by transcription. It is also the message
	// [isUsageError] already maps to ExitUsage, and a future rewording upstream carries to both at once.
	refusal := cobra.NoArgs(command, operands)
	if refusal == nil {
		return nil
	}

	// Printed here, not by the caller. Cobra prints its own errors inside ExecuteC; this refusal happens
	// before Execute is ever called, so nothing downstream would ever show it -- and a bare exit 64 tells a
	// reader no more than the silent exit 0 it replaced.
	refused := fmt.Errorf("%w%s", refusal, suggestionsFor(command, operands[0]))
	fmt.Fprintf(root.ErrOrStderr(), "%s %s\n", root.ErrPrefix(), refused)                  //nolint:errcheck // a refusal that cannot be written still refuses
	fmt.Fprintf(root.ErrOrStderr(), "Run '%v --help' for usage.\n", command.CommandPath()) //nolint:errcheck // as above

	return ExitWith(ExitUsage, refused)
}

// suggestionsFor renders the "Did you mean this?" block cobra appends to a root's refusal.
//
// `findSuggestions` is unexported, but it is only this formatting around [cobra.Command.SuggestionsFor], which
// is exported -- so a group's refusal reads exactly as the root's does, spacing included, rather than merely
// similarly.
//
// Parameters:
//   - `command`: the group whose verbs are the candidates.
//   - `typed`: what the user typed where a verb belongs.
//
// Returns:
//   - `string`: the block, or empty when no verb is close enough.
func suggestionsFor(command *cobra.Command, typed string) string {

	if command.DisableSuggestions {
		return ""
	}

	// `findSuggestions` defaults the distance before asking; [cobra.Command.SuggestionsFor] does not, and on a
	// fresh command the field is 0, so without this only an exact or prefix match would ever be offered.
	if command.SuggestionsMinimumDistance <= 0 {
		command.SuggestionsMinimumDistance = 2
	}

	suggestions := command.SuggestionsFor(typed)
	if len(suggestions) == 0 {
		return ""
	}

	var block strings.Builder
	block.WriteString("\n\nDid you mean this?\n")
	for _, suggestion := range suggestions {
		fmt.Fprintf(&block, "\t%v\n", suggestion) //nolint:errcheck // a strings.Builder write cannot fail
	}

	return block.String()
}

// operandsOf returns the arguments that are not flags, nor a flag's value.
//
// Cobra's `stripFlags` does this and is unexported. The rule it implements, and this one: `--name=value` and
// `-abc` are flags; `--name value` and `-n value` consume the next argument unless the flag is boolean, which
// takes no value; everything after `--` is an operand.
//
// `writ repo --output json` must reach help rather than be refused for the verb `json`, which is why the
// boolean question is asked of the command's own flags rather than assumed.
//
// Parameters:
//   - `command`: the resolved command, whose flags say which take values.
//   - `arguments`: the leftovers [cobra.Command.Find] returned.
//
// Returns:
//   - `[]string`: the operands, in order.
func operandsOf(command *cobra.Command, arguments []string) []string {

	flags := command.Flags()
	operands := make([]string, 0, len(arguments))

	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]

		switch {
		case argument == "--":
			return append(operands, arguments[index+1:]...)

		case strings.HasPrefix(argument, "--"):
			name, _, assigned := strings.Cut(strings.TrimPrefix(argument, "--"), "=")
			if !assigned && takesAValue(flags.Lookup(name)) {
				index++
			}

		case strings.HasPrefix(argument, "-") && argument != "-":
			shorthand := strings.TrimPrefix(argument, "-")
			if !strings.Contains(shorthand, "=") && len(shorthand) == 1 &&
				takesAValue(flags.ShorthandLookup(shorthand)) {
				index++
			}

		default:
			operands = append(operands, argument)
		}
	}

	return operands
}

// takesAValue reports whether a flag consumes the argument after it.
//
// An unknown flag is assumed to take one: refusing `--nosuchflag value` as the verb `value` would be a worse
// answer than letting cobra report the unknown flag itself, which it does with the right exit code.
//
// Parameters:
//   - `flag`: the looked-up flag, or nil when the name is unknown.
//
// Returns:
//   - `bool`: true when the next argument belongs to this flag.
func takesAValue(flag *pflag.Flag) bool {

	if flag == nil {
		return true
	}

	return flag.NoOptDefVal == ""
}

// endregion
