import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Divider, Modal, Popover, Tag, Tooltip, message } from 'antd';
import {
  CopyOutlined,
  DownloadOutlined,
  EyeOutlined,
  QrcodeOutlined,
  ReloadOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';

import { ClipboardManager, HttpUtil, IntlUtil, SizeFormatter } from '@/utils';
import { formatInboundLabel } from '@/lib/inbounds/label';
import {
  normalizeClientIps,
  normalizeIPLimitEvents,
  type ClientIpInfo,
  type IPLimitEvent,
} from '@/lib/clients/ip-log';
import { useDatepicker } from '@/hooks/useDatepicker';
import type { ClientRecord, InboundOption } from '@/hooks/useClients';
import { isPostQuantumLink } from '@/lib/xray/inbound-link';
import { LinkTags, linkMetaText, parseLinkParts } from '@/lib/xray/link-label';
import { QrPanel } from '@/pages/inbounds/qr';
import ConfigBlock from '@/components/clients/ConfigBlock';
import { JsonEditor } from '@/components/form';
import { buildWireguardClientConfig, findWireguardInbound, isWireguardClient } from './wireguardConfig';
import './ClientInfoModal.css';

const INBOUND_PROTOCOL_COLORS: Record<string, string> = {
  vless: 'blue',
  vmess: 'geekblue',
  trojan: 'volcano',
  shadowsocks: 'magenta',
  hysteria: 'cyan',
  hysteria2: 'green',
  wireguard: 'gold',
  http: 'purple',
  mixed: 'lime',
  tunnel: 'orange',
};

const INBOUND_CHIP_LIMIT = 1;

interface SubSettings {
  enable: boolean;
  subURI: string;
  subJsonURI: string;
  subJsonEnable: boolean;
  subHappURI: string;
  subHappEnable: boolean;
  subClashURI: string;
  subClashEnable: boolean;
  publicHost?: string;
}

interface ClientInfoModalProps {
  open: boolean;
  client: ClientRecord | null;
  inboundsById: Record<number, InboundOption>;
  isOnline: boolean;
  subSettings?: SubSettings;
  onOpenChange: (open: boolean) => void;
}

interface ApiMsg<T = unknown> {
  success?: boolean;
  msg?: string;
  obj?: T;
}

interface SubscriptionProfile {
  status?: string;
  lastGeneratedAt?: number;
  lastValidatedAt?: number;
  lastError?: string;
}

interface HappPreview {
  document?: unknown;
  partial?: boolean;
  warnings?: string[];
}

const DEFAULT_SUB: SubSettings = {
  enable: false,
  subURI: '',
  subJsonURI: '',
  subJsonEnable: false,
  subHappURI: '',
  subHappEnable: false,
  subClashURI: '',
  subClashEnable: false,
  publicHost: '',
};

export default function ClientInfoModal({
  open,
  client,
  inboundsById,
  isOnline,
  subSettings = DEFAULT_SUB,
  onOpenChange,
}: ClientInfoModalProps) {
  const { datepicker } = useDatepicker();
  const { t } = useTranslation();
  const expiryLabel = (ts?: number) => {
    if (!ts) return '∞';
    if (ts < 0) {
      const days = Math.round(ts / -86400000);
      return `${t('pages.clients.delayedStart')}: ${days}d`;
    }
    return IntlUtil.formatDate(ts, datepicker);
  };
  const dateLabel = (ts?: number) => (!ts || ts <= 0 ? '-' : IntlUtil.formatDate(ts, datepicker));
  const [messageApi, messageContextHolder] = message.useMessage();
  const [links, setLinks] = useState<string[]>([]);
  const [clientIps, setClientIps] = useState<ClientIpInfo[]>([]);
  const [ipLimitEvents, setIPLimitEvents] = useState<IPLimitEvent[]>([]);
  const [ipsLoading, setIpsLoading] = useState(false);
  const [ipsClearing, setIpsClearing] = useState(false);
  const [ipsModalOpen, setIpsModalOpen] = useState(false);
  const [subscriptionProfile, setSubscriptionProfile] = useState<SubscriptionProfile | null>(null);
  const [happPreview, setHappPreview] = useState('');
  const [happPreviewOpen, setHappPreviewOpen] = useState(false);
  const [happLoading, setHappLoading] = useState(false);

  useEffect(() => {
    if (!open) {
      setLinks([]);
      setClientIps([]);
      setIPLimitEvents([]);
      setIpsModalOpen(false);
      setSubscriptionProfile(null);
      setHappPreview('');
      setHappPreviewOpen(false);
      return;
    }
    if (!client?.subId) return;
    let cancelled = false;
    (async () => {
      const [msg, profileMsg] = await Promise.all([
        HttpUtil.get(`/panel/api/clients/subLinks/${encodeURIComponent(client.subId!)}`),
        HttpUtil.get(
          `/panel/api/clients/${encodeURIComponent(client.email)}/subscriptionProfile`,
          undefined,
          { silent: true },
        ),
      ]) as [ApiMsg<string[]>, ApiMsg<SubscriptionProfile>];
      if (cancelled) return;
      setLinks(msg?.success && Array.isArray(msg.obj) ? msg.obj : []);
      setSubscriptionProfile(profileMsg?.success && profileMsg.obj ? profileMsg.obj : null);
    })();
    return () => { cancelled = true; };
  }, [open, client?.email, client?.subId]);

  const traffic = client?.traffic || null;
  const totalBytes = client?.totalGB || 0;
  const used = (traffic?.up || 0) + (traffic?.down || 0);
  const remaining = useMemo(() => {
    if (totalBytes <= 0) return -1;
    const r = totalBytes - used;
    return r > 0 ? r : 0;
  }, [totalBytes, used]);

  const subLink = useMemo(() => {
    if (!client?.subId || !subSettings?.subURI) return '';
    return subSettings.subURI + client.subId;
  }, [client?.subId, subSettings?.subURI]);

  const subJsonLink = useMemo(() => {
    if (!client?.subId) return '';
    if (!subSettings?.subJsonEnable || !subSettings?.subJsonURI) return '';
    return subSettings.subJsonURI + client.subId;
  }, [client?.subId, subSettings?.subJsonEnable, subSettings?.subJsonURI]);

  const subClashLink = useMemo(() => {
    if (!client?.subId) return '';
    if (!subSettings?.subClashEnable || !subSettings?.subClashURI) return '';
    return subSettings.subClashURI + client.subId;
  }, [client?.subId, subSettings?.subClashEnable, subSettings?.subClashURI]);

  const subHappLink = useMemo(() => {
    if (!client?.subId) return '';
    if (!subSettings?.subHappEnable || !subSettings?.subHappURI) return '';
    return subSettings.subHappURI + client.subId;
  }, [client?.subId, subSettings?.subHappEnable, subSettings?.subHappURI]);

  const showSubscription = !!(subSettings?.enable && client?.subId);
  const wgInbound = useMemo(() => findWireguardInbound(client, inboundsById), [client, inboundsById]);
  const wgConfigText = useMemo(() => {
    if (!client || !wgInbound || !isWireguardClient(client)) return '';
    return buildWireguardClientConfig(client, wgInbound, window.location.hostname, subSettings?.publicHost ?? '');
  }, [client, wgInbound, subSettings?.publicHost]);

  async function copyValue(text: string) {
    if (!text) return;
    const ok = await ClipboardManager.copyText(String(text));
    if (ok) messageApi.success(t('copied'));
  }

  async function loadIps() {
    if (!client?.email) return;
    setIpsLoading(true);
    try {
      const encoded = encodeURIComponent(client.email);
      const [msg, eventsMsg] = await Promise.all([
        HttpUtil.post(`/panel/api/clients/ips/${encoded}`),
        HttpUtil.get(`/panel/api/clients/${encoded}/ipLimitEvents`, undefined, { silent: true }),
      ]) as [ApiMsg<unknown[]>, ApiMsg<unknown[]>];
      if (!msg?.success) { setClientIps([]); return; }
      setClientIps(normalizeClientIps(msg.obj));
      setIPLimitEvents(eventsMsg?.success ? normalizeIPLimitEvents(eventsMsg.obj) : []);
    } finally {
      setIpsLoading(false);
    }
  }

  async function clearIps() {
    if (!client?.email) return;
    setIpsClearing(true);
    try {
      const msg = await HttpUtil.post(`/panel/api/clients/clearIps/${encodeURIComponent(client.email)}`) as ApiMsg;
      if (msg?.success) setClientIps([]);
    } finally {
      setIpsClearing(false);
    }
  }

  function openIpsModal() {
    setIpsModalOpen(true);
    if (clientIps.length === 0) void loadIps();
  }

  async function generateHapp(force: boolean): Promise<string> {
    if (!client?.email) return '';
    setHappLoading(true);
    try {
      const action = force ? 'happRegenerate' : 'happPreview';
      const path = `/panel/api/clients/${encodeURIComponent(client.email)}/${action}`;
      const msg = (force ? await HttpUtil.post(path) : await HttpUtil.get(path)) as ApiMsg<HappPreview>;
      if (!msg?.success || msg.obj?.document === undefined) {
        messageApi.error(msg?.msg || t('somethingWentWrong'));
        return '';
      }
      const content = JSON.stringify(msg.obj.document, null, 2);
      setHappPreview(content);
      setHappPreviewOpen(true);
      const profileMsg = await HttpUtil.get(
        `/panel/api/clients/${encodeURIComponent(client.email)}/subscriptionProfile`,
        undefined,
        { silent: true },
      ) as ApiMsg<SubscriptionProfile>;
      if (profileMsg?.success && profileMsg.obj) setSubscriptionProfile(profileMsg.obj);
      if (msg.obj.partial) {
        messageApi.warning(t('happ.partial', { count: msg.obj.warnings?.length ?? 0 }));
      } else {
        messageApi.success(t('happ.valid'));
      }
      return content;
    } finally {
      setHappLoading(false);
    }
  }

  async function downloadHapp() {
    const content = happPreview || await generateHapp(false);
    if (!content) return;
    const blob = new Blob([content], { type: 'application/json;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = `${client?.email || 'subscription'}.json`;
    anchor.click();
    URL.revokeObjectURL(url);
  }

  return (
    <>
      {messageContextHolder}
      <Modal
        open={open}
        title={client ? `${t('pages.clients.clientInfo')} — ${client.email}` : t('pages.clients.clientInfo')}
        footer={null}
        width={640}
        onCancel={() => onOpenChange(false)}
      >
        {client && (
          <>
            <table className="info-table block">
              <tbody>
                <tr>
                  <td>{t('pages.clients.online')}</td>
                  <td>
                    {client.enable && isOnline
                      ? <Tag color="green">{t('pages.clients.online')}</Tag>
                      : <Tag>{t('pages.clients.offline')}</Tag>}
                    <span className="hint">{t('lastOnline')}: {dateLabel(traffic?.lastOnline)}</span>
                  </td>
                </tr>
                <tr>
                  <td>{t('status')}</td>
                  <td>
                    <Tag color={client.enable ? 'green' : 'default'}>
                      {client.enable ? t('enabled') : t('disabled')}
                    </Tag>
                  </td>
                </tr>
                <tr>
                  <td>{t('pages.clients.email')}</td>
                  <td>
                    {client.email
                      ? <Tag color="green">{client.email}</Tag>
                      : <Tag color="red">{t('none')}</Tag>}
                  </td>
                </tr>
                <tr>
                  <td>{t('pages.clients.subId')}</td>
                  <td>
                    <Tag className="info-large-tag">{client.subId || '-'}</Tag>
                    {client.subId && (
                      <Button size="small" type="text" icon={<CopyOutlined />} aria-label={t('copy')} onClick={() => copyValue(client.subId!)} />
                    )}
                  </td>
                </tr>
                {client.uuid && (
                  <tr>
                    <td>{t('pages.clients.uuid')}</td>
                    <td>
                      <Tag className="info-large-tag">{client.uuid}</Tag>
                      <Button size="small" type="text" icon={<CopyOutlined />} aria-label={t('copy')} onClick={() => copyValue(client.uuid!)} />
                    </td>
                  </tr>
                )}
                {client.password && (
                  <tr>
                    <td>{t('password')}</td>
                    <td>
                      <Tag className="info-large-tag">{client.password}</Tag>
                      <Button size="small" type="text" icon={<CopyOutlined />} aria-label={t('copy')} onClick={() => copyValue(client.password!)} />
                    </td>
                  </tr>
                )}
                {client.auth && (
                  <tr>
                    <td>{t('pages.clients.auth')}</td>
                    <td>
                      <Tag className="info-large-tag">{client.auth}</Tag>
                      <Button size="small" type="text" icon={<CopyOutlined />} aria-label={t('copy')} onClick={() => copyValue(client.auth!)} />
                    </td>
                  </tr>
                )}
                <tr>
                  <td>{t('pages.clients.flow')}</td>
                  <td>
                    {client.flow ? <Tag>{client.flow}</Tag> : <Tag color="orange">{t('none')}</Tag>}
                  </td>
                </tr>
                <tr>
                  <td>{t('pages.inbounds.traffic')}</td>
                  <td>
                    <Tag>
                      ↑ {SizeFormatter.sizeFormat(traffic?.up || 0)}
                      {' '}/ ↓ {SizeFormatter.sizeFormat(traffic?.down || 0)}
                    </Tag>
                    <span className="hint">
                      {SizeFormatter.sizeFormat(used)} / {totalBytes > 0 ? SizeFormatter.sizeFormat(totalBytes) : '∞'}
                    </span>
                  </td>
                </tr>
                <tr>
                  <td>{t('remained')}</td>
                  <td>
                    {remaining < 0
                      ? <Tag color="purple">∞</Tag>
                      : <Tag color={remaining > 0 ? '' : 'red'}>{SizeFormatter.sizeFormat(remaining)}</Tag>}
                  </td>
                </tr>
                <tr>
                  <td>{t('pages.inbounds.expireDate')}</td>
                  <td>
                    {!client.expiryTime
                      ? <Tag color="purple">∞</Tag>
                      : <Tag color={client.expiryTime < 0 ? 'blue' : undefined}>{expiryLabel(client.expiryTime)}</Tag>}
                    {(client.expiryTime ?? 0) > 0 && (
                      <span className="hint">{IntlUtil.formatRelativeTime(client.expiryTime)}</span>
                    )}
                  </td>
                </tr>
                <tr>
                  <td>{t('pages.clients.ipLimit')}</td>
                  <td>{!client.limitIp ? <Tag>∞</Tag> : <Tag>{client.limitIp}</Tag>}</td>
                </tr>
                <tr>
                  <td>{t('pages.inbounds.IPLimitlog')}</td>
                  <td>
                    <Button size="small" icon={<EyeOutlined />} aria-label={t('pages.clients.ipLog')} loading={ipsLoading} onClick={openIpsModal}>
                      {clientIps.length > 0 ? clientIps.length : ''}
                    </Button>
                  </td>
                </tr>
                <tr>
                  <td>{t('pages.inbounds.createdAt')}</td>
                  <td><Tag>{dateLabel(client.createdAt)}</Tag></td>
                </tr>
                <tr>
                  <td>{t('pages.inbounds.updatedAt')}</td>
                  <td><Tag>{dateLabel(client.updatedAt)}</Tag></td>
                </tr>
                {client.group && (
                  <tr>
                    <td>{t('pages.clients.group')}</td>
                    <td><Tag color="geekblue">{client.group}</Tag></td>
                  </tr>
                )}
                {client.comment && (
                  <tr>
                    <td>{t('pages.clients.comment')}</td>
                    <td><Tag className="info-large-tag">{client.comment}</Tag></td>
                  </tr>
                )}
                <tr>
                  <td>{t('pages.clients.attachedInbounds')}</td>
                  <td>
                    {(() => {
                      const ids = client.inboundIds || [];
                      if (ids.length === 0) return <span className="hint">—</span>;
                      const visible = ids.slice(0, INBOUND_CHIP_LIMIT);
                      const overflow = ids.slice(INBOUND_CHIP_LIMIT);
                      const inboundChip = (id: number) => {
                        const ib = inboundsById[id];
                        const proto = (ib?.protocol || '').toLowerCase();
                        const color = INBOUND_PROTOCOL_COLORS[proto] ?? 'default';
                        const label = formatInboundLabel(ib?.tag, ib?.remark);
                        return (
                          <Tooltip key={id} title={label}>
                            <Tag color={color}>{label}</Tag>
                          </Tooltip>
                        );
                      };
                      return (
                        <div className="chips">
                          {visible.map((id) => inboundChip(id))}
                          {overflow.length > 0 && (
                            <Popover
                              trigger="click"
                              placement="bottomRight"
                              content={
                                <div className="chips chips-stack">
                                  {overflow.map((id) => inboundChip(id))}
                                </div>
                              }
                            >
                              <Tag color="default" className="chip-more">
                                +{overflow.length} {t('more') !== 'more' ? t('more') : 'more'}
                              </Tag>
                            </Popover>
                          )}
                        </div>
                      );
                    })()}
                  </td>
                </tr>
              </tbody>
            </table>

            {showSubscription && subLink && (
              <>
                <Divider>{t('subscription.title')}</Divider>
                <div className="link-row">
                  <Tag color="green" className="link-row-tag">SUB</Tag>
                  <a
                    href={subLink}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="link-row-title link-row-title-anchor"
                    title={subLink}
                  >
                    {client.subId}
                  </a>
                  <div className="link-row-actions">
                    <Tooltip title={t('copy')}>
                      <Button size="small" icon={<CopyOutlined />} aria-label={t('copy')} onClick={() => copyValue(subLink)} />
                    </Tooltip>
                    <Popover
                      trigger="click"
                      placement="left"
                      destroyOnHidden
                      content={<QrPanel value={subLink} remark={`${client.email} — ${t('subscription.title')}`} size={220} />}
                    >
                      <Tooltip title={t('pages.clients.qrCode')}>
                        <Button size="small" icon={<QrcodeOutlined />} aria-label={t('pages.clients.qrCode')} />
                      </Tooltip>
                    </Popover>
                  </div>
                </div>
                {subJsonLink && (
                  <div className="link-row">
                    <Tag color="purple" className="link-row-tag">JSON</Tag>
                    <a
                      href={subJsonLink}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="link-row-title link-row-title-anchor"
                      title={subJsonLink}
                    >
                      {client.subId}
                    </a>
                    <div className="link-row-actions">
                      <Tooltip title={t('copy')}>
                        <Button size="small" icon={<CopyOutlined />} aria-label={t('copy')} onClick={() => copyValue(subJsonLink)} />
                      </Tooltip>
                      <Popover
                        trigger="click"
                        placement="left"
                        destroyOnHidden
                        content={<QrPanel value={subJsonLink} remark={`${client.email} — JSON`} size={220} />}
                      >
                        <Tooltip title={t('pages.clients.qrCode')}>
                          <Button size="small" icon={<QrcodeOutlined />} aria-label={t('pages.clients.qrCode')} />
                        </Tooltip>
                      </Popover>
                    </div>
                  </div>
                )}
                {subClashLink && (
                  <div className="link-row">
                    <Tooltip title="Clash / Mihomo">
                      <Tag color="gold" className="link-row-tag">CLASH</Tag>
                    </Tooltip>
                    <a
                      href={subClashLink}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="link-row-title link-row-title-anchor"
                      title={subClashLink}
                    >
                      {client.subId}
                    </a>
                    <div className="link-row-actions">
                      <Tooltip title={t('copy')}>
                        <Button size="small" icon={<CopyOutlined />} aria-label={t('copy')} onClick={() => copyValue(subClashLink)} />
                      </Tooltip>
                      <Popover
                        trigger="click"
                        placement="left"
                        destroyOnHidden
                        content={<QrPanel value={subClashLink} remark={`${client.email} — Clash / Mihomo`} size={220} />}
                      >
                        <Tooltip title={t('pages.clients.qrCode')}>
                          <Button size="small" icon={<QrcodeOutlined />} aria-label={t('pages.clients.qrCode')} />
                        </Tooltip>
                      </Popover>
                    </div>
                  </div>
                )}
                {subHappLink && (
                  <div className="link-row">
                    <Tag color="cyan" className="link-row-tag">HAPP</Tag>
                    <a
                      href={subHappLink}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="link-row-title link-row-title-anchor"
                      title={subHappLink}
                    >
                      {client.subId}
                    </a>
                    <div className="link-row-actions">
                      <Tag color={subscriptionProfile?.status === 'valid' ? 'green' : subscriptionProfile?.status === 'partial' ? 'orange' : 'default'}>
                        {t(`happ.status.${subscriptionProfile?.status || 'pending'}`)}
                      </Tag>
                      <Tooltip title={t('copy')}>
                        <Button size="small" icon={<CopyOutlined />} aria-label={t('copy')} onClick={() => copyValue(subHappLink)} />
                      </Tooltip>
                      <Popover
                        trigger="click"
                        placement="left"
                        destroyOnHidden
                        content={<QrPanel value={subHappLink} remark={`${client.email} — Happ JSON`} size={220} />}
                      >
                        <Tooltip title={t('pages.clients.qrCode')}>
                          <Button size="small" icon={<QrcodeOutlined />} aria-label={t('pages.clients.qrCode')} />
                        </Tooltip>
                      </Popover>
                      <Tooltip title={t('happ.preview')}>
                        <Button size="small" icon={<EyeOutlined />} loading={happLoading} onClick={() => generateHapp(false)} />
                      </Tooltip>
                      <Tooltip title={t('happ.regenerate')}>
                        <Button size="small" icon={<ThunderboltOutlined />} loading={happLoading} onClick={() => generateHapp(true)} />
                      </Tooltip>
                      <Tooltip title={t('download')}>
                        <Button size="small" icon={<DownloadOutlined />} disabled={happLoading} onClick={downloadHapp} />
                      </Tooltip>
                    </div>
                  </div>
                )}
                {subHappLink && subscriptionProfile?.lastGeneratedAt ? (
                  <div className="hint">
                    {t('happ.lastChanged')}: {dateLabel(subscriptionProfile.lastGeneratedAt)}
                    {subscriptionProfile.lastError ? ` — ${subscriptionProfile.lastError}` : ''}
                  </div>
                ) : null}
              </>
            )}

            {links.length > 0 && (
              <>
                <Divider>{t('pages.inbounds.copyLink')}</Divider>
                {links.map((link, idx) => {
                  const parts = parseLinkParts(link);
                  const fallback = `${t('pages.clients.link')} ${idx + 1}`;
                  const rowTitle = (parts && linkMetaText(parts)) || fallback;
                  const qrRemark = parts?.remark || rowTitle;
                  const canQr = !isPostQuantumLink(link);
                  return (
                    <div key={idx} className="link-row">
                      {parts
                        ? <LinkTags parts={parts} />
                        : <Tag className="link-row-tag">LINK</Tag>}
                      <span className="link-row-title" title={rowTitle}>{rowTitle}</span>
                      <div className="link-row-actions">
                        <Tooltip title={t('copy')}>
                          <Button size="small" icon={<CopyOutlined />} aria-label={t('copy')} onClick={() => copyValue(link)} />
                        </Tooltip>
                        {canQr && (
                          <Popover
                            trigger="click"
                            placement="left"
                            destroyOnHidden
                            content={<QrPanel value={link} remark={qrRemark} size={220} />}
                          >
                            <Tooltip title={t('pages.clients.qrCode')}>
                              <Button size="small" icon={<QrcodeOutlined />} aria-label={t('pages.clients.qrCode')} />
                            </Tooltip>
                          </Popover>
                        )}
                      </div>
                    </div>
                  );
                })}
              </>
            )}

            {wgConfigText && client && (
              <>
                <Divider>{t('pages.clients.wireguardConfig')}</Divider>
                <ConfigBlock
                  label={t('pages.clients.config')}
                  text={wgConfigText}
                  fileName={`${client.email}.conf`}
                  qrRemark={client.email || 'peer'}
                />
              </>
            )}
          </>
        )}
      </Modal>

      <Modal
        open={happPreviewOpen}
        title={t('happ.preview')}
        width={860}
        footer={[
          <Button key="download" icon={<DownloadOutlined />} onClick={downloadHapp}>{t('download')}</Button>,
          <Button key="close" type="primary" onClick={() => setHappPreviewOpen(false)}>{t('close')}</Button>,
        ]}
        onCancel={() => setHappPreviewOpen(false)}
      >
        <JsonEditor value={happPreview} readOnly minHeight="360px" maxHeight="65vh" />
      </Modal>

      <Modal
        open={ipsModalOpen}
        title={`${t('pages.inbounds.IPLimitlog')}${client?.email ? ` — ${client.email}` : ''}`}
        width={440}
        onCancel={() => setIpsModalOpen(false)}
        footer={[
          <Button key="refresh" icon={<ReloadOutlined />} loading={ipsLoading} onClick={loadIps}>
            {t('refresh')}
          </Button>,
          <Button key="clear" danger loading={ipsClearing} disabled={clientIps.length === 0} onClick={clearIps}>
            {t('pages.clients.clearAll')}
          </Button>,
          <Button key="close" type="primary" onClick={() => setIpsModalOpen(false)}>
            {t('close')}
          </Button>,
        ]}
      >
        {clientIps.length > 0 ? (
          <div style={{ maxHeight: 360, overflowY: 'auto' }}>
            {clientIps.map((entry, idx) => (
              <Tag
                key={idx}
                color="blue"
                style={{
                  display: 'block',
                  width: 'fit-content',
                  maxWidth: '100%',
                  marginBottom: 6,
                  padding: '2px 8px',
                  fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
                }}
              >
                <span>{entry.ip}</span>
                {(entry.firstSeen || entry.lastSeen || entry.time) ? (
                  <span style={{ display: 'block', opacity: 0.75 }}>
                    {entry.firstSeen || entry.time} → {entry.lastSeen || entry.time}
                  </span>
                ) : null}
                {(entry.node || entry.inbound) ? (
                  <span style={{ display: 'block', opacity: 0.85, fontWeight: 600 }}>
                    {entry.node ? `@ ${entry.node}` : ''}{entry.node && entry.inbound ? ' · ' : ''}{entry.inbound}
                  </span>
                ) : null}
              </Tag>
            ))}
          </div>
        ) : (
          <Tag>{t('tgbot.noIpRecord')}</Tag>
        )}
        {ipLimitEvents.length > 0 ? (
          <div style={{ marginTop: 16, maxHeight: 180, overflowY: 'auto' }}>
            {ipLimitEvents.map((event, index) => (
              <Tag key={`${event.time}-${event.ip}-${index}`} color={event.action === 'ban' ? 'red' : 'green'} style={{ marginBottom: 6 }}>
                {event.time} · {event.action.toUpperCase()} · {event.ip}
              </Tag>
            ))}
          </div>
        ) : null}
      </Modal>
    </>
  );
}
