# Weekly search report

`.claude/agents/weekly-seo.md` is a Claude Code subagent that checks how zoomies.sh is doing in search once a week and writes a report here. It only reads and drafts: it never submits a disavow file, sends outreach, opens a pull request or edits the site.

Nothing runs it automatically. To run it, ask Claude Code to use the `weekly-seo` agent, or schedule that request (a Claude Code routine, or cron around `claude -p`) on a machine you trust. Pick the schedule yourself; the agent is deliberately not wired to one.

## What it needs

* `config.yml` in this directory. `openseo_project_id` stays empty until the site is verified in Search Console and connected in OpenSEO; until then the agent skips the Search Console tasks and says so in the report.
* The OpenRush and OpenSEO connectors for rankings, backlinks and AI citations. The tool names in the agent's front matter must match the connectors as they are named in your environment.
* `npx` for Lighthouse, and `curl`.

## Files it writes

| File | Contents | Rule |
| --- | --- | --- |
| `YYYY-MM-DD.md` | The week's report | One per run |
| `memory/stats.csv` | `date,indexed_pages,clicks_7d,impressions_7d,avg_position,new_backlinks,lost_backlinks,ai_overview_citations,lcp_ms,cls,performance_mobile,performance_desktop` | Append only |
| `memory/keywords.csv` | `date,keyword,position,impressions,clicks,ctr,page_url` | Append only |
| `memory/serp-snapshot.json` | Last week's top three results per target keyword | Overwritten each run |
| `memory/changelog.md` | Changes made to the site and what they did | Entries are never deleted |

Whether these files live in this repository or in a separate operations repository is your choice. They are small, but a weekly commit of a report is noise in this history, so a separate repository may suit better.

## Logging a change

When you change something to improve search results, add an entry so the agent can measure it later:

```markdown
## YYYY-MM-DD: short title

* **Page or scope**: `https://zoomies.sh/page/` or "site-wide"
* **Change**: one sentence
* **Hypothesis**: why it should help
* **Baseline**: clicks, position, page speed at the time
* **Metric to watch**: clicks, position, LCP, CLS or backlinks
* **Measure after**: YYYY-MM-DD, three to four weeks out
* **Status**: pending-validation
```

## What it deliberately leaves out

There is no analytics, so there is no AI-crawler traffic check and no UTM tracking. Add one only together with an update to [the privacy page](../docs/privacy.md).
