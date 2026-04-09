---
name: think-before-execute
description: On ambiguous or architectural tasks, ask before implementing. On direct tasks, execute.
---

## Rule

Before executing, decide which of the two modes you are in:

- **Direct mode** — the task is clear and bounded: execute.
- **Alignment mode** — the task has ambiguity, an architectural decision, or unspecified behavior change: present the approach and wait for approval before writing code.

Don't confuse the two. Asking permission for everything becomes friction and the founder loses patience. Executing an architectural decision without aligning generates rework and frustration.

## Criteria

**Direct mode** when:
- Clear, bounded instruction ("change this color to gold", "rename this file", "add this text here")
- Obvious bug fix with a known root cause
- Point adjustment to an existing feature (known what, where, how it works)
- You can describe the final diff in one sentence before starting

**Alignment mode** when:
- Task involves an architectural decision, pattern, or behavior the founder didn't specify
- More than one reasonable way to implement, with real trade-offs
- Task affects multiple files in a non-trivial way
- You need to "infer" what the founder meant at some point
- Task is vague ("make this better", "make it faster") with no metrics or criteria

## How to apply alignment mode

Don't write code. Write:

1. **Task summary** — how you understood it (1-2 sentences)
2. **Decision(s) in play** — what needs to be decided
3. **Options** — at least 2, with concrete pros and cons
4. **Recommendation** — which one you think is best and why
5. **Specific question for the founder** — what you need from them to move forward

Wait for an answer. Don't "start while they're still deciding" — if the founder changes direction, you will have thrown work away.

## Why the self-check matters

Claude (the model) has a tendency toward **direct mode by default**. It wants to solve, it wants to deliver. That works for 70% of tasks, but fails catastrophically on the 30% that need alignment — because the model "decides alone" on points where the founder was supposed to be the decider.

The self-check is not optional. It's the only tool you have to resist the execution bias.

## Examples

✅ **Direct mode:** "Change the primary button color to gold on the login screen." → go ahead.

⚠️ **Alignment mode:** "Change the app's primary color." → stop. "That affects 40 components. Primary is a CSS variable in index.css, I can change it there and it propagates. But there are also hardcoded uses in some places that need refactor. Do you want me to (a) just update the token and leave the hardcoded ones as follow-up, (b) do everything at once, or (c) something else?"

✅ **Direct mode:** "Add error logging to the `send-push` edge function." → go ahead.

⚠️ **Alignment mode:** "Improve the app's error handling." → stop. Too vague — scope, pattern, and location need to be defined.
