import { describe, expect, it } from 'vitest';

import { normalizeClientIps, normalizeIPLimitEvents } from '@/lib/clients/ip-log';

describe('client IP log normalization', () => {
  it('keeps first/last seen, node and inbound attribution', () => {
    expect(normalizeClientIps([{
      ip: '203.0.113.8',
      time: 'last',
      firstSeen: 'first',
      lastSeen: 'last',
      node: 'edge-1',
      inbound: 'vless-reality',
    }])).toEqual([{
      ip: '203.0.113.8',
      time: 'last',
      firstSeen: 'first',
      lastSeen: 'last',
      node: 'edge-1',
      inbound: 'vless-reality',
    }]);
  });

  it('filters malformed events and preserves exact BAN/UNBAN data', () => {
    expect(normalizeIPLimitEvents([
      { time: 'now', action: 'ban', ip: '198.51.100.2' },
      { action: 7, ip: 'bad' },
    ])).toEqual([{ time: 'now', action: 'ban', ip: '198.51.100.2' }]);
  });
});
