export const CLIENT_IMPORT_MAX_BYTES = 8 * 1024 * 1024;
export const CLIENT_IMPORT_MAX_ROWS = 1000;

export type ClientImportErrorCode =
  | 'expectedArray'
  | 'tooLarge'
  | 'tooManyRows'
  | 'invalidRow'
  | 'missingEmail'
  | 'duplicateEmail'
  | 'duplicateSubId'
  | 'invalidInboundIds';

export interface ClientImportPreviewRow {
  index: number;
  email: string;
  inboundCount: number;
  errors: ClientImportErrorCode[];
}

export interface ClientImportPreview {
  items: unknown[];
  rows: ClientImportPreviewRow[];
  parseError: string;
  errorCode?: ClientImportErrorCode;
}

function documentBytes(value: string): number {
  return new TextEncoder().encode(value).length;
}

function recordValue(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null;
  return value as Record<string, unknown>;
}

export function parseClientImport(value: string): ClientImportPreview {
  if (documentBytes(value) > CLIENT_IMPORT_MAX_BYTES) {
    return { items: [], rows: [], parseError: '', errorCode: 'tooLarge' };
  }

  let parsed: unknown;
  try {
    parsed = JSON.parse(value);
  } catch (error) {
    return {
      items: [],
      rows: [],
      parseError: error instanceof Error ? error.message : String(error),
    };
  }
  if (!Array.isArray(parsed)) {
    return { items: [], rows: [], parseError: '', errorCode: 'expectedArray' };
  }
  if (parsed.length > CLIENT_IMPORT_MAX_ROWS) {
    return { items: parsed, rows: [], parseError: '', errorCode: 'tooManyRows' };
  }

  const emailOwners = new Map<string, number>();
  const subIDOwners = new Map<string, number>();
  const rows = parsed.map((item, offset): ClientImportPreviewRow => {
    const index = offset + 1;
    const row = recordValue(item);
    const client = recordValue(row?.client);
    const email = typeof client?.email === 'string' ? client.email.trim() : '';
    const subID = typeof client?.subId === 'string' ? client.subId.trim() : '';
    const inboundIds = row?.inboundIds;
    const errors: ClientImportErrorCode[] = [];

    if (!row || !client) errors.push('invalidRow');
    if (!email) errors.push('missingEmail');
    const normalizedEmail = email.toLowerCase();
    if (normalizedEmail && emailOwners.has(normalizedEmail)) errors.push('duplicateEmail');
    if (normalizedEmail && !emailOwners.has(normalizedEmail)) emailOwners.set(normalizedEmail, index);
    if (subID && subIDOwners.has(subID)) errors.push('duplicateSubId');
    if (subID && !subIDOwners.has(subID)) subIDOwners.set(subID, index);
    if (!Array.isArray(inboundIds) || inboundIds.some((id) => !Number.isInteger(id) || Number(id) <= 0)) {
      errors.push('invalidInboundIds');
    }

    return {
      index,
      email,
      inboundCount: Array.isArray(inboundIds) ? inboundIds.length : 0,
      errors,
    };
  });

  return { items: parsed, rows, parseError: '' };
}
