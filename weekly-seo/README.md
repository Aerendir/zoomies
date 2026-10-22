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
| `memory/stats.csv` | `date,window_end,indexed_in_sample,sample_size,sitemap_urls,clicks_7d,impressions_7d,avg_position,new_backlinks,lost_backlinks,ai_overview_citations,lcp_ms,cls,performance_mobile,performance_desktop` | Append only |
| `memory/keywords.csv` | `date,keyword,position,impressions,clicks,ctr,page_url` | Append only |
| `memory/serp-snapshot.json` | Last week's top three results per target keyword | Overwritten each run |
| `memory/changelog.md` | Changes made to the site and what they did | Entries are never deleted |

`window_end` is the last date with complete Search Console data (data arrives 2 to 3 days late), and the `_7d` columns cover the seven days ending there. `indexed_in_sample` and `sample_size` come from the fixed `index_sample` list in `config.yml`; keep that list unchanged, or the weeks stop being comparable. `sitemap_urls` is pages submitted, not pages indexed.

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

The site counts page views with Cloudflare Web Analytics (cookieless; see [the privacy page](../docs/privacy.md)). It does not report AI-crawler traffic or UTM campaigns, and OpenSEO cannot read it, so read it in the Cloudflare dashboard. Add any other tracker only together with an update to the privacy page.
