<script lang="ts">
  export interface Metric {
    label: string;
    value: string;
    detail: string;
    href?: string;
    /**
     * Further places the figure can be opened, each its own link: the failures
     * behind a count, say. Set above the tile's own link, which covers the whole
     * tile, so that a click on one of these goes to it and not to the tile's.
     */
    links?: { text: string; href: string }[];
    progress?: number | null;
    tone?: 'neutral' | 'success' | 'danger' | 'warning' | 'accent' | 'busy';
  }
  /**
   * `strip` is the figures as one row of a single card, each a label, a number and one
   * line of detail (the whole of it in the title), for a page whose list is what it is
   * for and whose figures are context.
   */
  let {
    items,
    compact = false,
    strip = false,
  }: { items: Metric[]; compact?: boolean; strip?: boolean } = $props();
</script>

<dl class="metrics" class:compact class:strip>
  {#each items as item (item.label)}
    <div class="metric" data-tone={item.tone ?? 'neutral'} title={strip ? item.detail : undefined}>
      <dt>
        {#if item.href}<a
            href={item.href}
            aria-label={`${item.label}: ${item.value}. ${item.detail}`}
            >{item.label}<span aria-hidden="true">↗</span></a
          >{:else}{item.label}{/if}
      </dt>
      <dd class="value">{item.value}</dd>
      <!-- A second description of the same term. Only dt and dd may sit in a
           definition list's groups: the detail used to be a bare paragraph and
           the meter a bare div, and a screen reader lost the pairing. -->
      <dd class="detail">
        {item.detail}
        {#if item.links?.length}<ul class="links">
            {#each item.links as link (link.href)}<li>
                <a href={link.href}>{link.text}</a>
              </li>{/each}
          </ul>{/if}
        {#if item.progress != null}<div class="meter" aria-hidden="true">
            <span style:width={`${Math.min(100, Math.max(0, item.progress))}%`}></span>
          </div>{/if}
      </dd>
    </div>
  {/each}
</dl>

<style>
  .metrics {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(155px, 1fr));
    gap: var(--z-space-3);
    margin: 0 0 var(--z-space-5);
  }
  .metrics.compact {
    grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
    gap: var(--z-space-2);
    margin-bottom: 0;
  }
  .metrics.compact .metric {
    padding: var(--z-space-3);
  }
  .metrics.compact .value {
    margin: var(--z-space-1) 0;
    font-size: var(--z-text-xl);
  }
  .metrics.compact .detail {
    font-size: var(--z-text-2xs);
    line-height: var(--z-leading-2xs);
  }
  .metrics.strip {
    display: flex;
    flex-wrap: wrap;
    gap: 0;
    margin-bottom: var(--z-space-4);
    border: var(--z-border-width) solid var(--z-border);
    border-radius: var(--z-radius-xl);
    background: var(--z-surface);
    overflow: hidden;
  }
  .metrics.strip .metric {
    flex: 1 1 11rem;
    padding: var(--z-space-3) var(--z-space-4);
    border: 0;
    border-radius: 0;
    background: none;
    box-shadow: inset var(--z-border-width) 0 0 var(--z-border);
  }
  .metrics.strip .value {
    margin: var(--z-space-1) 0;
    font-size: var(--z-text-xl);
  }
  .metrics.strip .detail {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .metrics.strip .metric:has(a):hover {
    background: var(--z-surface-hover);
  }
  .metric {
    position: relative;
    min-width: 0;
    padding: var(--z-space-4);
    border: var(--z-border-width) solid var(--z-border);
    border-radius: var(--z-radius-md);
    background: var(--z-surface);
  }
  dt {
    font-size: var(--z-text-xs);
    color: var(--z-text-muted);
  }
  dd {
    margin: 0;
  }
  .value {
    margin: var(--z-space-2) 0;
    font-size: var(--z-text-2xl);
    font-weight: var(--z-weight-semibold);
    font-variant-numeric: tabular-nums;
    letter-spacing: var(--z-tracking-tight);
  }
  .detail {
    color: var(--z-text-muted);
    font-size: var(--z-text-xs);
  }
  [data-tone='success'] .value {
    color: var(--z-idle);
  }
  [data-tone='danger'] .value {
    color: var(--z-danger);
  }
  [data-tone='warning'] .value {
    color: var(--z-pending);
  }
  a {
    color: inherit;
    text-decoration: none;
    display: flex;
    justify-content: space-between;
    gap: var(--z-space-2);
  }
  a::after {
    content: '';
    position: absolute;
    inset: 0;
    border-radius: var(--z-radius-md);
  }
  .links {
    margin: var(--z-space-2) 0 0;
    padding: 0;
    list-style: none;
  }
  /* Above the tile's stretched link, or the tile's own would take the click. */
  .links a {
    position: relative;
    display: inline;
    color: var(--z-accent);
    text-decoration: underline;
  }
  .links a::after {
    content: none;
  }
  .metric:has(a):hover {
    border-color: var(--z-accent-border);
    background: var(--z-surface-hover);
  }
  .metric:focus-within {
    outline: 2px solid var(--z-accent);
    outline-offset: 2px;
  }
  [data-tone='accent'] .value {
    color: var(--z-accent);
  }
  [data-tone='busy'] .value {
    color: var(--z-busy);
  }
  .meter {
    height: var(--z-space-1);
    background: var(--z-surface-sunken);
    border-radius: var(--z-radius-full);
    overflow: hidden;
    margin-top: var(--z-space-3);
  }
  .meter span {
    display: block;
    height: 100%;
    background: var(--z-accent);
  }
</style>
