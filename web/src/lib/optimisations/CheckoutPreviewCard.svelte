<script lang="ts">
  import type { CheckoutPreview } from '../api/types';
  import { checkoutSeconds } from './input';
  import RelativeTime from '../components/RelativeTime.svelte';

  let { preview }: { preview: CheckoutPreview } = $props();
  const p = $derived(preview.preview);
</script>

<section class="card" aria-labelledby="preview-title">
  <h2 id="preview-title">Retained preview</h2>
  <p>
    {p.explanation}
  </p>
  <dl class="facts">
    <div>
      <dt>Preview</dt>
      <dd class="mono">{preview.id}</dd>
    </div>
    <div>
      <dt>Repository</dt>
      <dd>{preview.repository}</dd>
    </div>
    <div>
      <dt>File</dt>
      <dd>{p.path}, line {p.line}</dd>
    </div>
    <div>
      <dt>Commit</dt>
      <dd class="mono">{p.head_sha}</dd>
    </div>
    <div>
      <dt>Run</dt>
      <dd>{p.run_id}, attempt {p.attempt}</dd>
    </div>
    <div>
      <dt>Checkout step took</dt>
      <dd>{checkoutSeconds(p.checkout_ms)} (descriptive, not a saving)</dd>
    </div>
    <div>
      <dt>Kept</dt>
      <dd><RelativeTime value={preview.created_at} /> by {preview.reviewed_by}</dd>
    </div>
    <div>
      <dt>Verification</dt>
      <dd>{p.verification}: no measurement shows this change helps</dd>
    </div>
    <div>
      <dt>Currentness</dt>
      <dd>Not checked: the commit and your settings were not re-read</dd>
    </div>
  </dl>
  <div class="diff" role="group" aria-label="The one line this preview changes, untrusted text">
    <pre class="removed"><span aria-hidden="true">- </span><span class="sr"
        >Removed: </span>{p.before.replace(/\n$/, '')}</pre>
    <pre class="added"><span aria-hidden="true">+ </span><span class="sr"
        >Added: </span>{p.after.replace(/\n$/, '')}</pre>
  </div>
  <p>
    Nothing was published. There is no pull request, no approval and no job run. Keeping a preview
    does not apply it.
  </p>
</section>

<style>
  .card {
    padding: var(--z-space-6);
    background: var(--z-surface);
    border: var(--z-border-width) solid var(--z-border);
    border-radius: var(--z-radius-lg);
    min-width: 0;
  }
  h2 {
    font-size: var(--z-text-lg);
    font-weight: var(--z-weight-semibold);
    margin-block: var(--z-space-4);
  }
  p {
    margin-block: var(--z-space-3);
  }
  .facts {
    display: grid;
    gap: var(--z-space-2);
  }
  .facts div {
    display: flex;
    flex-wrap: wrap;
    gap: var(--z-space-2);
  }
  dt {
    font-weight: var(--z-weight-semibold);
  }
  dd {
    overflow-wrap: anywhere;
    min-width: 0;
  }
  .mono {
    font-family: var(--z-font-mono);
  }
  .diff {
    margin-block: var(--z-space-4);
    overflow: auto;
    border: var(--z-border-width) solid var(--z-border);
    border-radius: var(--z-radius-md);
  }
  pre {
    font-family: var(--z-font-mono);
    font-size: var(--z-text-sm);
    padding: var(--z-space-2) var(--z-space-4);
    white-space: pre;
    margin: 0;
  }
  .added {
    font-weight: var(--z-weight-semibold);
    background: var(--z-surface-sunken);
  }
  .sr {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
  }
</style>
