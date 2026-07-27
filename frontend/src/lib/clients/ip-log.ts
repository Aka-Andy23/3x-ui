// Shape of one entry in a client's IP log, as returned by
// POST /panel/api/clients/ips/:email. `node` is the name of the node the IP is
// connecting through, or '' when it is on this local panel (or unattributed).
export type ClientIpInfo = {
  ip: string;
  time: string;
  firstSeen: string;
  lastSeen: string;
  node: string;
  inbound: string;
};

export type IPLimitEvent = {
  time: string;
  action: 'ban' | 'unban' | string;
  ip: string;
};

export function normalizeIPLimitEvents(obj: unknown): IPLimitEvent[] {
  if (!Array.isArray(obj)) return [];
  return obj.flatMap((value) => {
    if (!value || typeof value !== 'object') return [];
    const row = value as Record<string, unknown>;
    if (typeof row.ip !== 'string' || typeof row.action !== 'string') return [];
    return [{
      ip: row.ip,
      action: row.action,
      time: typeof row.time === 'string' ? row.time : '',
    }];
  });
}

// normalizeClientIps accepts the API payload and returns typed entries. It also
// tolerates the legacy shape (a plain array of "ip (time)" strings) so the UI
// keeps working against older panels.
export function normalizeClientIps(obj: unknown): ClientIpInfo[] {
  if (!Array.isArray(obj)) return [];
  const out: ClientIpInfo[] = [];
  for (const x of obj) {
    if (typeof x === 'string') {
      if (x.length > 0) out.push({ ip: x, time: '', firstSeen: '', lastSeen: '', node: '', inbound: '' });
      continue;
    }
    if (x && typeof x === 'object') {
      const o = x as Record<string, unknown>;
      const ip = typeof o.ip === 'string' ? o.ip : '';
      if (!ip) continue;
      out.push({
        ip,
        time: typeof o.time === 'string' ? o.time : '',
        firstSeen: typeof o.firstSeen === 'string' ? o.firstSeen : '',
        lastSeen: typeof o.lastSeen === 'string' ? o.lastSeen : '',
        node: typeof o.node === 'string' ? o.node : '',
        inbound: typeof o.inbound === 'string' ? o.inbound : '',
      });
    }
  }
  return out;
}
