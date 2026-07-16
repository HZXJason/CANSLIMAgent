# Staged confirmatory extension

The confirmatory study requests a maximum of 45 non-overlapping MU decision
nodes from 2022-01-03 through 2025-06-30, each with a complete 20-session
outcome. The runner freezes at least 550 calendar days of prehistory before the
first realized node for price and indicator tools; the actual eligible node
dates and row bounds are immutable fields in `protocol.json`.

Stage one evaluates 30 evenly distributed nodes in both blind and
quant-informed contexts. Five evenly distributed stage-one nodes receive one
additional repeat per context solely to measure model stochasticity. Repeats
are averaged within their market node and never increase the inferential sample
size. Stage one therefore uses 70 graph runs.

If stage one is inconclusive, stage two evaluates the remaining 15 independent
nodes in both contexts, for a maximum total of 100 graph runs. Both interim
looks use a two-sided alpha of 0.025, equivalent to a 97.5% confidence interval,
so the two-look familywise error rate is at most 0.05 by Bonferroni. The same
97.5% interval must fit inside +/-1 percentage point per 20-day node to declare
practical equivalence.

Positive, negative, and practically-null decisions retain the paired and
context-consistency gates in `PROTOCOL.md`. If the maximum sample remains
inconclusive, the scientific result remains inconclusive; the product decision
may still reject deployment because a costly architecture has not met its
predeclared burden of proof.
