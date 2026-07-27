import { describe, expect, it } from 'vitest';

import { parseClientImport } from '@/lib/clients/import-preview';

describe('client import preview', () => {
  it('accepts the portable export shape', () => {
    const result = parseClientImport(JSON.stringify([
      { client: { email: 'alice', subId: 'sub-a' }, inboundIds: [1, 2] },
      { client: { email: 'bob', subId: 'sub-b' }, inboundIds: [] },
    ]));

    expect(result.parseError).toBe('');
    expect(result.rows).toEqual([
      { index: 1, email: 'alice', inboundCount: 2, errors: [] },
      { index: 2, email: 'bob', inboundCount: 0, errors: [] },
    ]);
  });

  it('reports duplicate identities and malformed rows', () => {
    const result = parseClientImport(JSON.stringify([
      { client: { email: 'Alice', subId: 'same' }, inboundIds: [1] },
      { client: { email: 'alice', subId: 'same' }, inboundIds: ['bad'] },
      { inboundIds: [] },
    ]));

    expect(result.rows[1]?.errors).toEqual(['duplicateEmail', 'duplicateSubId', 'invalidInboundIds']);
    expect(result.rows[2]?.errors).toEqual(['invalidRow', 'missingEmail']);
  });

  it('rejects a non-array document', () => {
    expect(parseClientImport('{}').errorCode).toBe('expectedArray');
  });
});
