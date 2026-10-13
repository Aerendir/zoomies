import assert from 'node:assert/strict';
import { test } from 'node:test';
import { uploadInstanceArchive } from '../src/lib/settings/transfer-upload.ts';
const chunkBytes = 8 * 1024 * 1024;
function fixture(size: number, lostResponse = false) {
  let offset = 0,
    chunks = 0,
    probes = 0,
    verifying = false;
  const sizes: number[] = [];
  const snapshot = () => ({
    id: 'upload_test',
    size,
    offset,
    chunk_bytes: chunkBytes,
    state: verifying ? 'verifying' : 'uploading',
  });
  const saved = globalThis.fetch;
  globalThis.fetch = async (url, init) => {
    const path = String(url);
    if (path.endsWith('/transfers/uploads')) {
      assert.equal(JSON.parse(String(init?.body)).size, size);
      return Response.json(snapshot(), { status: 201 });
    }
    if (path.includes('/chunks?')) {
      assert.equal(new URL(path, 'http://localhost').searchParams.get('offset'), String(offset));
      assert.ok(init?.body instanceof Blob);
      assert.ok(init.body.size <= chunkBytes);
      sizes.push(init.body.size);
      offset += init.body.size;
      chunks++;
      if (lostResponse && chunks === 1) throw new TypeError('response lost after saving bytes');
      return Response.json(snapshot());
    }
    if (path.endsWith('/verify')) {
      assert.equal(offset, size);
      assert.equal(JSON.parse(String(init?.body)).passphrase, 'archive passphrase');
      verifying = true;
      return Response.json(snapshot(), { status: 202 });
    }
    assert.ok(path.endsWith('/upload_test'));
    probes++;
    return Response.json(
      verifying
        ? { ...snapshot(), state: 'verified', backup: { id: 'verified_copy' } }
        : snapshot(),
    );
  };
  return {
    restore: () => {
      globalThis.fetch = saved;
    },
    sizes,
    probes: () => probes,
  };
}
test('a large instance archive uses bounded requests and polls its verified receipt', async () => {
  const size = 106910 * 1024,
    fake = fixture(size);
  try {
    const result = await uploadInstanceArchive(
      new File([new Uint8Array(size)], 'instance.zbk'),
      'archive passphrase',
      null,
      () => {},
    );
    assert.equal(result.id, 'verified_copy');
    assert.equal(fake.sizes.length, 14);
    assert.equal(
      fake.sizes.reduce((sum, n) => sum + n, 0),
      size,
    );
    assert.equal(fake.probes(), 1);
  } finally {
    fake.restore();
  }
});
test('a lost chunk response resumes from the durable offset without sending it twice', async () => {
  const size = chunkBytes + 5,
    fake = fixture(size, true);
  try {
    const result = await uploadInstanceArchive(
      new File([new Uint8Array(size)], 'instance.zbk'),
      'archive passphrase',
      null,
      () => {},
    );
    assert.equal(result.id, 'verified_copy');
    assert.deepEqual(fake.sizes, [chunkBytes, 5]);
    assert.equal(fake.probes(), 2);
  } finally {
    fake.restore();
  }
});
test('failed verification never returns an importable backup', async () => {
  const saved = globalThis.fetch;
  globalThis.fetch = async () =>
    Response.json({
      id: 'failed',
      size: 1,
      offset: 1,
      chunk_bytes: chunkBytes,
      state: 'failed',
      error: 'Check the archive passphrase.',
    });
  try {
    await assert.rejects(
      uploadInstanceArchive(
        new File(['x'], 'instance.zbk'),
        'wrong phrase',
        { id: 'failed', size: 1, offset: 1, chunk_bytes: chunkBytes, state: 'failed' },
        () => {},
      ),
      /Check the archive passphrase/,
    );
  } finally {
    globalThis.fetch = saved;
  }
});
