import {
  ApiError,
  createTransferUpload,
  getTransferUpload,
  uploadTransferChunk,
  verifyTransferUpload,
} from '../api/client';
import type { Schemas } from '../api/types';

/** Keep the receipt between attempts so a lost response does not require re-uploading. */
export async function uploadInstanceArchive(
  file: File,
  passphrase: string,
  previous: Schemas['TransferUpload'] | null,
  update: (value: Schemas['TransferUpload']) => void,
): Promise<Schemas['Backup']> {
  let upload = previous
    ? await getTransferUpload(previous.id)
    : await createTransferUpload(file.size);
  update(upload);
  while (upload.offset < file.size && upload.state === 'uploading') {
    const end = Math.min(upload.offset + upload.chunk_bytes, file.size);
    const chunk = file.slice(upload.offset, end);
    // A request can have saved its bytes even when its response was lost.
    // Ask for the durable offset before retrying that same bounded chunk.
    let received = false;
    for (let attempt = 0; attempt < 3; attempt++) {
      try {
        upload = await uploadTransferChunk(upload.id, upload.offset, chunk);
        received = true;
        break;
      } catch (cause) {
        if (!(cause instanceof ApiError) || (cause.status > 0 && cause.status < 500)) throw cause;
        upload = await getTransferUpload(upload.id);
        if (upload.offset === end) {
          received = true;
          break;
        }
        if (attempt === 2) throw cause;
      }
    }
    if (!received) throw new Error('The upload did not complete. Try again.');
    update(upload);
  }
  if (upload.state === 'uploading') upload = await verifyTransferUpload(upload.id, passphrase);
  update(upload);
  while (upload.state === 'verifying') {
    await new Promise((resolve) => setTimeout(resolve, 1000));
    upload = await getTransferUpload(upload.id);
    update(upload);
  }
  if (upload.state !== 'verified' || !upload.backup) {
    throw new Error(upload.error || 'The archive was not verified. Choose it again to retry.');
  }
  return upload.backup;
}
