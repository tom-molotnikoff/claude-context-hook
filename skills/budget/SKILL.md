---
name: budget
description: Turn on the context check for this session with a stop threshold, then work through a batch of tasks by the stopping rule and end with a handoff note. Use when the operator runs /ctx:budget <threshold>.
argument-hint: <threshold>
allowed-tools:
  - Bash(ctx)
  - Bash(ctx *)
---

# /ctx:budget

Threshold: `$ARGUMENTS`

## Turn the check on

Do this before any other work.

1. If no threshold was given above, ask the operator for one (a whole number from 10 to 90). Don't start work until the check is on.
2. Run `ctx arm <threshold>`.
3. If `ctx arm` rejects the threshold, ask the operator for a threshold and don't start work until `ctx arm` succeeds.

## The stopping rule

The check can send you two messages, each marked `[ctx]`: an early warning at two thirds of the threshold, and a stop message at the threshold.

- The first task always starts.
- Until the early warning, work normally.
- After the early warning, before each new task:
  1. Run `ctx` for the current figure. It shows current usage and starting usage as percentages of the window.
  2. Work out average = (current - starting usage) / tasks done.
  3. Scale the average by how big the next task looks next to the ones already done: bigger than usual, scale it up; smaller, scale it down.
  4. Start the task only if current + that estimate stays at or under the threshold. Otherwise stop and write the handoff note.
- After the stop message, finish the current task and start nothing new. Then write the handoff note.

## What the check can't do

- It is advisory. It can't stop you by itself, so following the rule is up to you.
- `/clear` turns the check off.

## Handoff note

End your final message with a handoff note containing:

- Tasks done.
- Tasks not started.
- Anything the next session needs to know.
- The average cost per task as a percentage of the window: (current - starting usage) / tasks done, using a final `ctx` figure.
