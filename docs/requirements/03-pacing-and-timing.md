# 03 — Pacing and timing model

## Units
- A **paragraph** is the unit of reading and timing. Its words are counted on the visible text.
- **Plan wpm** (`P`): the pace saved with the script (or fitted to a target).
- **Live wpm** (`L`): the pace the prompter is scrolling at right now (changed with ↑/↓ or A).

## Planned durations
For paragraph *i* with `w_i` words and a pause `p_i` seconds after it:

```
read_i  = w_i * 60 / P
plan_i  = read_i + p_i
start_i = sum(plan_0 .. plan_{i-1})
total   = sum(plan_i)
```

If the script has a target time `T`, Fit to time sets `P = words * 60 / (T - pauses)`
(rounded to the nearest whole wpm, clamped to 60–300).

## Scroll position (stage 1, clock-driven)
The prompter keeps a **script clock** `s` measured in plan-seconds. While playing,
`s += dt * L / P` every frame; during a pause marker the clock advances at real time.
The vertical offset maps the clock to layout: inside paragraph *i*,
`y = top_i + clamp((s - start_i) / read_i, 0, 1) * height_i`, so the paragraph crosses the focus
line in exactly `read_i` plan-seconds at the plan pace. Jumps set `s = start_j`.
The wheel nudges `s` by ±0.5 s per notch.

## Actual time per paragraph
Real elapsed time is attributed to the paragraph whose range contains `s`. Paused time counts
(you are still talking or thinking). Skipped paragraphs are marked as skipped, not as zero.

## Behind / ahead
```
elapsed  = real time since the countdown ended
expected = s                       # plan-seconds already covered
deficit  = elapsed - expected      # > 0 means behind
remainingWords = words not yet covered
remainingPause = pauses not yet covered
available      = T - elapsed - remainingPause      # T = target, or total if no target
neededWpm      = remainingWords * 60 / available
```
- Show the **behind** alert when `deficit > 3 s` and `neededWpm` ≤ 300; show
  "can't finish on time — cut a paragraph" when `available ≤ 0` or `neededWpm > 300`.
- Show **ahead** when `deficit < -5 s`.
- Pressing **A** sets `L = round(neededWpm)`.
- Hysteresis: the alert clears only when `deficit < 1 s` to avoid flicker.

## Projected finish
`projected = elapsed + (total - s) * P / L` — shown next to the target.

## Stage 3 change
In smart-follow mode `s` comes from speech alignment instead of the clock; every formula above
stays the same, which is why the model is expressed in plan-seconds.
