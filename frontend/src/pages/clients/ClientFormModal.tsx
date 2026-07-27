import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  AutoComplete,
  Alert,
  Button,
  Card,
  Col,
  Divider,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Row,
  Select,
  Space,
  Steps,
  Switch,
  Tabs,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd';
import {
  ArrowDownOutlined,
  ArrowUpOutlined,
  DeleteOutlined,
  EyeOutlined,
  PlusOutlined,
  ReloadOutlined,
  RetweetOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import type { Dayjs } from 'dayjs';
import { FormProvider, useForm, useWatch, useFieldArray } from 'react-hook-form';

import { HttpUtil, RandomUtil, Wireguard } from '@/utils';
import { formatInboundLabel } from '@/lib/inbounds/label';
import { generateMtprotoSecret } from '@/lib/xray/inbound-defaults';
import {
  normalizeClientIps,
  normalizeIPLimitEvents,
  type ClientIpInfo,
  type IPLimitEvent,
} from '@/lib/clients/ip-log';
import { DateTimePicker, SelectAllClearButtons } from '@/components/form';
import { JsonEditor } from '@/components/form';
import { FormField } from '@/components/form/rhf';
import { TLS_FLOW_CONTROL } from '@/schemas/primitives';
import type { ClientRecord, InboundOption, ExternalLink, ExternalLinkInput } from '@/hooks/useClients';
import { useFail2banStatusQuery, getLimitIpNotice } from '@/api/queries/useFail2banStatusQuery';
import { ClientFormSchema, ClientCreateFormSchema, type ClientFormValues } from '@/schemas/client';

const FLOW_OPTIONS = Object.values(TLS_FLOW_CONTROL);
const VMESS_SECURITY_OPTIONS = ['auto', 'aes-128-gcm', 'chacha20-poly1305'] as const;

const MULTI_CLIENT_PROTOCOLS = new Set([
  'shadowsocks', 'vless', 'vmess', 'trojan', 'hysteria', 'wireguard', 'mtproto',
]);

const CLIENT_FORM_MODAL_Z_INDEX = 1000;
const CLIENT_IP_LOG_MODAL_Z_INDEX = CLIENT_FORM_MODAL_Z_INDEX + 1;

interface ExternalLinkRow {
  id?: number;
  kind: 'link' | 'subscription' | 'json' | 'json_subscription';
  value: string;
  remark: string;
  comment: string;
  enabled: boolean;
  priority: number;
  updateIntervalMinutes: number;
  timeoutSeconds: number;
  maxResponseBytes: number;
  maxRedirects: number;
  lastSuccessAt?: number;
  lastAttemptAt?: number;
  lastError?: string;
  lastHttpStatus?: number;
  createdAt?: number;
  updatedAt?: number;
}

interface SubscriptionProfileValues {
  enabled: boolean;
  displayName: string;
  language: string;
  title: string;
  linkExpiresAt: number;
  updateInterval: number;
  autoSelectEnabled: boolean;
  autoSelectName: string;
  autoSelectCandidates: string[];
  probeUrl: string;
  probeTimeoutSeconds: number;
  probeIntervalSeconds: number;
  fallbackTag: string;
  switchThresholdMs: number;
  debounceSeconds: number;
}

interface DirectDomainInput {
  value: string;
  mode: 'include' | 'exclude';
  comment: string;
  enabled: boolean;
}

interface ApiMsg<T = unknown> {
  success?: boolean;
  msg?: string;
  obj?: T;
}

interface DirectDomainRecord {
  mode?: 'include' | 'exclude';
  displayDomain?: string;
  domain?: string;
}

interface ClientNodeOption {
  id: number;
  name?: string;
  status?: string;
  enable?: boolean;
}

type Mode = 'add' | 'edit';

interface SaveMetaEdit {
  isEdit: true;
  email: string;
  attach: number[];
  detach: number[];
  externalLinks: ExternalLinkInput[];
  subscriptionProfile: SubscriptionProfileValues;
  directIncludes: string;
  directExcludes: string;
}

interface SaveMetaCreate {
  isEdit: false;
  email: string;
  externalLinks: ExternalLinkInput[];
  subscriptionProfile: SubscriptionProfileValues;
  directIncludes: string;
  directExcludes: string;
}

interface SaveCreatePayload {
  client: Record<string, unknown>;
  clientEnable: boolean;
  inboundIds: number[];
  externalLinks: ExternalLinkInput[];
  subscriptionProfile: SubscriptionProfileValues;
  directDomains: DirectDomainInput[];
}

interface ClientFormModalProps {
  open: boolean;
  mode: Mode;
  client: ClientRecord | null;
  inbounds: InboundOption[];
  nodes?: ClientNodeOption[];
  attachedExternalLinks?: ExternalLink[];
  attachedIds?: number[];
  tgBotEnable?: boolean;
  groups?: string[];
  save: (
    payload: Record<string, unknown> | SaveCreatePayload,
    meta: SaveMetaEdit | SaveMetaCreate,
  ) => Promise<ApiMsg | null>;
  resetTraffic?: (client: ClientRecord) => Promise<ApiMsg | null>;
  onOpenChange: (open: boolean) => void;
}

type Values = ClientFormValues & {
  expiryDate: number;
  externalLinks: ExternalLinkRow[];
  subscriptionProfile: SubscriptionProfileValues;
  directIncludes: string;
  directExcludes: string;
  wgPrivateKey: string;
  wgPublicKey: string;
  wgPreSharedKey: string;
  wgAllowedIPs: string;
  secret: string;
  adTag: string;
};

const EMPTY: Values = {
  email: '',
  subId: '',
  uuid: '',
  password: '',
  auth: '',
  flow: '',
  security: 'auto',
  reverseTag: '',
  totalGB: 0,
  expiryDate: 0,
  delayedStart: false,
  delayedDays: 0,
  reset: 0,
  limitIp: 0,
  tgId: 0,
  group: '',
  comment: '',
  enable: true,
  inboundIds: [],
  externalLinks: [],
  subscriptionProfile: {
    enabled: true,
    displayName: '',
    language: 'en',
    title: '',
    linkExpiresAt: 0,
    updateInterval: 60,
    autoSelectEnabled: false,
    autoSelectName: '',
    autoSelectCandidates: [],
    probeUrl: 'https://www.gstatic.com/generate_204',
    probeTimeoutSeconds: 5,
    probeIntervalSeconds: 300,
    fallbackTag: '',
    switchThresholdMs: 0,
    debounceSeconds: 0,
  },
  directIncludes: '',
  directExcludes: '',
  wgPrivateKey: '',
  wgPublicKey: '',
  wgPreSharedKey: '',
  wgAllowedIPs: '',
  secret: '',
  adTag: '',
};

function toExternalLinkRows(links: ExternalLink[] | undefined): ExternalLinkRow[] {
  return (links || []).map((l) => ({
    id: l.id,
    kind: l.kind,
    value: l.value || '',
    remark: l.remark || '',
    comment: l.comment || '',
    enabled: l.enabled !== false,
    priority: l.priority || 0,
    updateIntervalMinutes: l.updateIntervalMinutes || 60,
    timeoutSeconds: l.timeoutSeconds || 8,
    maxResponseBytes: l.maxResponseBytes || 2097152,
    maxRedirects: l.maxRedirects ?? 3,
    lastSuccessAt: l.lastSuccessAt,
    lastAttemptAt: l.lastAttemptAt,
    lastError: l.lastError,
    lastHttpStatus: l.lastHttpStatus,
    createdAt: l.createdAt,
    updatedAt: l.updatedAt,
  }));
}

function bytesToGB(bytes: number): number {
  if (!bytes || bytes <= 0) return 0;
  return Math.round((bytes / (1024 * 1024 * 1024)) * 100) / 100;
}

function gbToBytes(gb: number): number {
  if (!gb || gb <= 0) return 0;
  return Math.round(gb * 1024 * 1024 * 1024);
}

export default function ClientFormModal({
  open,
  mode,
  client,
  inbounds,
  nodes = [],
  attachedExternalLinks = [],
  attachedIds = [],
  tgBotEnable = false,
  groups = [],
  save,
  resetTraffic,
  onOpenChange,
}: ClientFormModalProps) {
  const { t } = useTranslation();
  const [messageApi, messageContextHolder] = message.useMessage();
  const isEdit = mode === 'edit';

  const methods = useForm<Values>({ defaultValues: EMPTY });
  const inboundIds = useWatch({ control: methods.control, name: 'inboundIds' });
  const delayedStart = useWatch({ control: methods.control, name: 'delayedStart' });
  const expiryDate = useWatch({ control: methods.control, name: 'expiryDate' });
  const enable = useWatch({ control: methods.control, name: 'enable' });
  const flow = useWatch({ control: methods.control, name: 'flow' });
  const reverseTag = useWatch({ control: methods.control, name: 'reverseTag' });
  const secret = useWatch({ control: methods.control, name: 'secret' });
  const email = useWatch({ control: methods.control, name: 'email' });
  const uuid = useWatch({ control: methods.control, name: 'uuid' });
  const password = useWatch({ control: methods.control, name: 'password' });
  const subId = useWatch({ control: methods.control, name: 'subId' });
  const auth = useWatch({ control: methods.control, name: 'auth' });
  const wgPrivateKey = useWatch({ control: methods.control, name: 'wgPrivateKey' });
  const limitIp = useWatch({ control: methods.control, name: 'limitIp' });
  const watchedExternalLinks = useWatch({ control: methods.control, name: 'externalLinks' });
  const externalLinks = useMemo(() => watchedExternalLinks || [], [watchedExternalLinks]);
  const subscriptionProfile = useWatch({ control: methods.control, name: 'subscriptionProfile' });
  const directIncludes = useWatch({ control: methods.control, name: 'directIncludes' });
  const directExcludes = useWatch({ control: methods.control, name: 'directExcludes' });
  const {
    fields: externalLinkFields,
    append: appendExternalLink,
    remove: removeExternalLink,
    move: moveExternalLink,
  } = useFieldArray({ control: methods.control, name: 'externalLinks' });

  const [submitting, setSubmitting] = useState(false);
  const [currentStep, setCurrentStep] = useState(0);
  const [nodeFilter, setNodeFilter] = useState<number | 'all'>('all');
  const [protocolFilter, setProtocolFilter] = useState('all');
  const [resetting, setResetting] = useState(false);
  const [clientIps, setClientIps] = useState<ClientIpInfo[]>([]);
  const [ipLimitEvents, setIPLimitEvents] = useState<IPLimitEvent[]>([]);
  const [ipsLoading, setIpsLoading] = useState(false);
  const [ipsClearing, setIpsClearing] = useState(false);
  const [ipsModalOpen, setIpsModalOpen] = useState(false);
  const fail2ban = useFail2banStatusQuery();
  const limitIpDisabled = !fail2ban.usable;
  const limitIpNotice = getLimitIpNotice(fail2ban, t);

  function addExternalLinkRow(kind: ExternalLinkRow['kind']) {
    appendExternalLink({
      kind,
      value: kind === 'json' ? '{\n  "outbounds": []\n}' : '',
      remark: '',
      comment: '',
      enabled: true,
      priority: 0,
      updateIntervalMinutes: 60,
      timeoutSeconds: 8,
      maxResponseBytes: 2097152,
      maxRedirects: 3,
    });
  }

  useEffect(() => {
    if (!open) return;
    setIpsModalOpen(false);
    setIPLimitEvents([]);
    setCurrentStep(0);
    setNodeFilter('all');
    setProtocolFilter('all');

    if (isEdit && client) {
      const et = Number(client.expiryTime) || 0;
      const seed: Values = {
        ...EMPTY,
        email: client.email || '',
        subId: client.subId || '',
        uuid: client.uuid || '',
        password: client.password || '',
        auth: client.auth || '',
        flow: client.flow || '',
        security: !client.security || client.security === 'none' || client.security === 'zero'
          ? 'auto'
          : client.security,
        reverseTag: client.reverse?.tag || '',
        totalGB: bytesToGB(client.totalGB || 0),
        reset: Number(client.reset) || 0,
        limitIp: client.limitIp || 0,
        tgId: Number(client.tgId) || 0,
        group: client.group || '',
        comment: client.comment || '',
        enable: !!client.enable,
        inboundIds: Array.isArray(attachedIds) ? [...attachedIds] : [],
        externalLinks: toExternalLinkRows(attachedExternalLinks),
        wgPrivateKey: client.privateKey || '',
        wgPublicKey: client.publicKey || '',
        wgPreSharedKey: client.preSharedKey || '',
        wgAllowedIPs: client.allowedIPs || '',
        secret: client.secret || '',
        adTag: client.adTag || '',
      };
      if (et < 0) {
        seed.delayedStart = true;
        seed.delayedDays = Math.round(et / -86400000);
        seed.expiryDate = 0;
      } else {
        seed.delayedStart = false;
        seed.delayedDays = 0;
        seed.expiryDate = et > 0 ? et : 0;
      }
      methods.reset(seed);
      void loadSubscriptionData(client.email);
      void loadIps();
    } else {
      const wgKeypair = Wireguard.generateKeypair();
      methods.reset({
        ...EMPTY,
        email: RandomUtil.randomLowerAndNum(10),
        uuid: RandomUtil.randomUUID(),
        subId: RandomUtil.randomLowerAndNum(16),
        password: RandomUtil.randomLowerAndNum(16),
        auth: RandomUtil.randomLowerAndNum(16),
        wgPrivateKey: wgKeypair.privateKey,
        wgPublicKey: wgKeypair.publicKey,
      });
    }

    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, isEdit]);

  const flowCapableIds = useMemo(() => {
    const ids = new Set<number>();
    for (const row of inbounds || []) {
      if (row?.tlsFlowCapable) ids.add(row.id);
    }
    return ids;
  }, [inbounds]);

  const vlessLikeIds = useMemo(() => {
    const ids = new Set<number>();
    for (const row of inbounds || []) {
      if (row && row.protocol === 'vless') ids.add(row.id);
    }
    return ids;
  }, [inbounds]);

  const vmessIds = useMemo(() => {
    const ids = new Set<number>();
    for (const row of inbounds || []) {
      if (row && row.protocol === 'vmess') ids.add(row.id);
    }
    return ids;
  }, [inbounds]);

  const wireguardIds = useMemo(() => {
    const ids = new Set<number>();
    for (const row of inbounds || []) {
      if (row && row.protocol === 'wireguard') ids.add(row.id);
    }
    return ids;
  }, [inbounds]);

  const mtprotoIds = useMemo(() => {
    const ids = new Set<number>();
    for (const row of inbounds || []) {
      if (row && row.protocol === 'mtproto') ids.add(row.id);
    }
    return ids;
  }, [inbounds]);

  const mtprotoDomain = useMemo(() => {
    for (const id of inboundIds || []) {
      const ib = (inbounds || []).find((row) => row.id === id);
      if (ib?.protocol === 'mtproto' && ib.mtprotoDomain) return ib.mtprotoDomain;
    }
    return 'www.cloudflare.com';
  }, [inboundIds, inbounds]);

  const ss2022Method = useMemo(() => {
    for (const id of inboundIds || []) {
      const ib = (inbounds || []).find((row) => row.id === id);
      const method = ib?.ssMethod;
      if (method && method.substring(0, 4) === '2022') return method;
    }
    return '';
  }, [inboundIds, inbounds]);

  function regeneratePassword() {
    methods.setValue('password', ss2022Method
      ? RandomUtil.randomShadowsocksPassword(ss2022Method)
      : RandomUtil.randomLowerAndNum(16));
  }

  const showFlow = useMemo(
    () => (inboundIds || []).some((id) => flowCapableIds.has(id)),
    [inboundIds, flowCapableIds],
  );

  const showReverseTag = useMemo(
    () => (inboundIds || []).some((id) => vlessLikeIds.has(id)),
    [inboundIds, vlessLikeIds],
  );

  const showSecurity = useMemo(
    () => (inboundIds || []).some((id) => vmessIds.has(id)),
    [inboundIds, vmessIds],
  );

  const showWireguard = useMemo(
    () => (inboundIds || []).some((id) => wireguardIds.has(id)),
    [inboundIds, wireguardIds],
  );

  const showMtproto = useMemo(
    () => (inboundIds || []).some((id) => mtprotoIds.has(id)),
    [inboundIds, mtprotoIds],
  );

  function regenerateWireguardKeys() {
    const kp = Wireguard.generateKeypair();
    methods.setValue('wgPrivateKey', kp.privateKey);
    methods.setValue('wgPublicKey', kp.publicKey);
  }

  function regenerateMtprotoSecret() {
    methods.setValue('secret', generateMtprotoSecret(mtprotoDomain));
  }

  useEffect(() => {
    if (!showFlow && flow) {
      methods.setValue('flow', '');
    }
  }, [showFlow, flow, methods]);

  useEffect(() => {
    if (!showReverseTag && reverseTag) {
      methods.setValue('reverseTag', '');
    }
  }, [showReverseTag, reverseTag, methods]);

  useEffect(() => {
    if (!ss2022Method) return;
    const current = methods.getValues('password');
    if (!RandomUtil.isShadowsocks2022Password(current, ss2022Method)) {
      methods.setValue('password', RandomUtil.randomShadowsocksPassword(ss2022Method));
    }
  }, [ss2022Method, methods]);

  useEffect(() => {
    if (showMtproto && !secret) {
      methods.setValue('secret', generateMtprotoSecret(mtprotoDomain));
    }
  }, [showMtproto, secret, mtprotoDomain, methods]);

  const nodeByID = useMemo(
    () => new Map(nodes.map((node) => [node.id, node])),
    [nodes],
  );

  const inboundOptions = useMemo(
    () => (inbounds || [])
      .filter((ib) => MULTI_CLIENT_PROTOCOLS.has(ib.protocol || ''))
      .filter((ib) => ib.enable || (inboundIds || []).includes(ib.id))
      .map((ib) => {
        const node = ib.nodeId ? nodeByID.get(ib.nodeId) : undefined;
        const nodeName = ib.nodeId
          ? (node?.name || t('clientWizard.node', { id: ib.nodeId }))
          : t('clientWizard.localNode');
        const status = ib.nodeId ? (node?.status || 'unknown') : 'online';
        const unavailable = ib.nodeId
          ? node?.enable === false || status !== 'online'
          : false;
        return {
          label: `${nodeName} · ${ib.protocol || '-'} · ${formatInboundLabel(ib.tag, ib.remark)} · ${t(`pages.nodes.statusValues.${status}`)}`,
          value: ib.id,
          title: formatInboundLabel(ib.tag, ib.remark),
          nodeId: ib.nodeId || 0,
          nodeName,
          protocol: ib.protocol || '',
          disabled: unavailable && !(inboundIds || []).includes(ib.id),
        };
      }),
    [inbounds, inboundIds, nodeByID, t],
  );

  const visibleInboundOptions = useMemo(
    () => inboundOptions.filter((option) => (
      (nodeFilter === 'all' || option.nodeId === nodeFilter)
      && (protocolFilter === 'all' || option.protocol === protocolFilter)
    )),
    [inboundOptions, nodeFilter, protocolFilter],
  );

  const selectableInboundOptions = useMemo(
    () => visibleInboundOptions.filter((option) => !option.disabled),
    [visibleInboundOptions],
  );

  const groupedInboundOptions = useMemo(() => {
    const groups = new Map<string, typeof visibleInboundOptions>();
    for (const option of visibleInboundOptions) {
      const group = `${option.nodeName} · ${option.protocol || '-'}`;
      groups.set(group, [...(groups.get(group) || []), option]);
    }
    return [...groups.entries()].map(([label, options]) => ({ label, options }));
  }, [visibleInboundOptions]);

  const nodeFilterOptions = useMemo(() => {
    const available = new Map<number, string>();
    for (const option of inboundOptions) {
      available.set(option.nodeId, option.nodeName);
    }
    return [
      { value: 'all' as const, label: t('clientWizard.allNodes') },
      ...[...available.entries()].map(([value, label]) => ({ value, label })),
    ];
  }, [inboundOptions, t]);

  const protocolFilterOptions = useMemo(() => [
    { value: 'all', label: t('clientWizard.allProtocols') },
    ...[...new Set(inboundOptions.map((option) => option.protocol).filter(Boolean))]
      .sort()
      .map((protocol) => ({ value: protocol, label: protocol })),
  ], [inboundOptions, t]);

  const autoCandidateOptions = useMemo(() => {
    const local = (inbounds || [])
      .filter((ib) => (inboundIds || []).includes(ib.id))
      .map((ib) => ({
        value: `local:${ib.id}`,
        label: `${ib.protocol || '-'} · ${formatInboundLabel(ib.tag, ib.remark)}`,
      }));
    const external = externalLinks
      .filter((row) => row.id && row.enabled)
      .map((row) => ({
        value: `${row.kind === 'json' || row.kind === 'json_subscription' ? 'json' : 'external'}:${row.id}`,
        label: row.remark || row.comment || `${t('externalJson.source')} ${row.id}`,
      }));
    return [...local, ...external];
  }, [inbounds, inboundIds, externalLinks, t]);

  const expiryDayjs = useMemo<Dayjs | null>(
    () => (expiryDate > 0 ? dayjs(expiryDate) : null),
    [expiryDate],
  );

  const linkRows = externalLinkFields
    .map((field, index) => ({ field, index }))
    .filter((row) => row.field.kind === 'link');
  const subscriptionRows = externalLinkFields
    .map((field, index) => ({ field, index }))
    .filter((row) => row.field.kind === 'subscription');
  const manualJsonRows = externalLinkFields
    .map((field, index) => ({ field, index }))
    .filter((row) => row.field.kind === 'json');
  const remoteJsonRows = externalLinkFields
    .map((field, index) => ({ field, index }))
    .filter((row) => row.field.kind === 'json_subscription');

  async function loadSubscriptionData(clientEmail: string) {
    const encoded = encodeURIComponent(clientEmail);
    const [profileMsg, domainMsg] = await Promise.all([
      HttpUtil.get(`/panel/api/clients/${encoded}/subscriptionProfile`, undefined, { silent: true }),
      HttpUtil.get(`/panel/api/directDomains/list?clientEmail=${encoded}`, undefined, { silent: true }),
    ]) as [ApiMsg<SubscriptionProfileValues>, ApiMsg<DirectDomainRecord[]>];
    if (profileMsg?.success && profileMsg.obj) {
      methods.setValue('subscriptionProfile', {
        ...EMPTY.subscriptionProfile,
        ...profileMsg.obj,
      });
    }
    if (domainMsg?.success && Array.isArray(domainMsg.obj)) {
      const values = (mode: DirectDomainRecord['mode']) => domainMsg.obj!
        .filter((row) => row.mode === mode)
        .map((row) => row.displayDomain || row.domain || '')
        .filter(Boolean)
        .join('\n');
      methods.setValue('directIncludes', values('include'));
      methods.setValue('directExcludes', values('exclude'));
    }
  }

  function validateManualJSON(index: number, format = false) {
    try {
      const parsed = JSON.parse(methods.getValues(`externalLinks.${index}.value`) || '');
      if (format) {
        methods.setValue(`externalLinks.${index}.value`, JSON.stringify(parsed, null, 2));
      }
      messageApi.success(t('externalJson.valid'));
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : t('externalJson.invalid'));
    }
  }

  async function refreshRemoteSource(index: number) {
    const row = methods.getValues(`externalLinks.${index}`);
    if (!row?.id) return;
    const msg = await HttpUtil.post(`/panel/api/clients/externalSources/${row.id}/refresh`) as ApiMsg<ExternalLink>;
    if (!msg?.success || !msg.obj) {
      messageApi.error(msg?.msg || t('somethingWentWrong'));
      return;
    }
    methods.setValue(`externalLinks.${index}`, toExternalLinkRows([msg.obj])[0]);
    messageApi.success(t('externalJson.refreshSuccess'));
  }

  async function loadIps() {
    if (!isEdit || !client?.email) return;
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

  function openIpsModal() {
    setIpsModalOpen(true);
    if (clientIps.length === 0) void loadIps();
  }

  async function clearIps() {
    if (!isEdit || !client?.email) return;
    setIpsClearing(true);
    try {
      const msg = await HttpUtil.post(`/panel/api/clients/clearIps/${encodeURIComponent(client.email)}`) as ApiMsg<{ failedNodes?: string[] }>;
      if (msg?.success) {
        setClientIps([]);
        setIPLimitEvents([]);
        const failedNodes = msg.obj?.failedNodes || [];
        if (failedNodes.length > 0) {
          messageApi.warning(`${t('somethingWentWrong')}: ${failedNodes.join(', ')}`);
        } else {
          messageApi.success(t('pages.inbounds.toasts.logCleanSuccess'));
        }
      }
    } finally {
      setIpsClearing(false);
    }
  }

  function close() {
    onOpenChange(false);
  }

  async function onResetTraffic() {
    if (!isEdit || !client?.email || !resetTraffic) return;
    setResetting(true);
    try {
      const msg = await resetTraffic(client);
      if (msg?.success) {
        messageApi.success(t('pages.clients.toasts.trafficReset'));
      } else {
        messageApi.error(msg?.msg || t('somethingWentWrong'));
      }
    } finally {
      setResetting(false);
    }
  }

  async function onSubmit() {
    const values = methods.getValues();
    const schema = isEdit ? ClientFormSchema : ClientCreateFormSchema;
    const validated = schema.safeParse({
      email: values.email,
      subId: values.subId,
      uuid: values.uuid,
      password: values.password,
      auth: values.auth,
      flow: values.flow,
      security: values.security,
      reverseTag: values.reverseTag,
      totalGB: values.totalGB,
      delayedStart: values.delayedStart,
      delayedDays: values.delayedDays,
      reset: values.reset,
      limitIp: values.limitIp,
      tgId: values.tgId,
      group: values.group,
      comment: values.comment,
      enable: values.enable,
      inboundIds: values.inboundIds,
    });
    if (!validated.success) {
      const issue = validated.error.issues[0];
      messageApi.error(t(issue?.message ?? 'somethingWentWrong'));
      return;
    }
    const expiryTime = values.delayedStart
      ? -86400000 * (Number(values.delayedDays) || 0)
      : (values.expiryDate || 0);
    const clientPayload: Record<string, unknown> = {
      email: values.email.trim(),
      subId: values.subId,
      id: values.uuid,
      password: values.password,
      auth: values.auth,
      flow: showFlow ? (values.flow || '') : '',
      security: showSecurity ? (values.security || 'auto') : 'auto',
      totalGB: gbToBytes(values.totalGB),
      expiryTime,
      reset: Number(values.reset) || 0,
      limitIp: Number(values.limitIp) || 0,
      tgId: Number(values.tgId) || 0,
      group: values.group,
      comment: values.comment,
      enable: !!values.enable,
    };
    const reverseTagValue = showReverseTag ? (values.reverseTag || '').trim() : '';
    if (reverseTagValue) {
      clientPayload.reverse = { tag: reverseTagValue };
    }

    if (showWireguard) {
      clientPayload.privateKey = values.wgPrivateKey;
      clientPayload.publicKey = values.wgPublicKey;
      if (values.wgPreSharedKey) {
        clientPayload.preSharedKey = values.wgPreSharedKey;
      }
      const allowedIPs = values.wgAllowedIPs
        .split(',')
        .map((s) => s.trim())
        .filter((s) => s !== '');
      if (allowedIPs.length > 0) {
        clientPayload.allowedIPs = allowedIPs;
      }
    }

    if (showMtproto) {
      const adTag = values.adTag.trim();
      if (adTag !== '' && !/^[0-9a-fA-F]{32}$/.test(adTag)) {
        messageApi.error(t('pages.inbounds.form.mtgAdTagInvalid'));
        return;
      }
      clientPayload.secret = values.secret;
      clientPayload.adTag = adTag;
    }

    const externalLinksPayload: ExternalLinkInput[] = values.externalLinks
      .map((r) => ({
        id: r.id,
        kind: r.kind,
        value: r.value.trim(),
        remark: r.remark.trim(),
        comment: r.comment.trim(),
        enabled: r.enabled,
        priority: Number(r.priority) || 0,
        updateIntervalMinutes: Number(r.updateIntervalMinutes) || 60,
        timeoutSeconds: Number(r.timeoutSeconds) || 8,
        maxResponseBytes: Number(r.maxResponseBytes) || 2097152,
        maxRedirects: Number(r.maxRedirects) || 0,
      }))
      .filter((r) => r.value !== '');
    const profilePayload: SubscriptionProfileValues = {
      ...values.subscriptionProfile,
      enabled: !!values.subscriptionProfile.enabled,
      displayName: values.subscriptionProfile.displayName.trim() || values.email.trim(),
      title: values.subscriptionProfile.title.trim() || values.email.trim(),
      language: values.subscriptionProfile.language.trim() || 'en',
      updateInterval: Number(values.subscriptionProfile.updateInterval) || 60,
      probeTimeoutSeconds: Number(values.subscriptionProfile.probeTimeoutSeconds) || 5,
      probeIntervalSeconds: Number(values.subscriptionProfile.probeIntervalSeconds) || 300,
      switchThresholdMs: Number(values.subscriptionProfile.switchThresholdMs) || 0,
      debounceSeconds: Number(values.subscriptionProfile.debounceSeconds) || 0,
    };
    const parseDirectDomains = (raw: string, mode: DirectDomainInput['mode']): DirectDomainInput[] => raw
      .split(/[\s,]+/)
      .map((value) => value.trim())
      .filter((value) => value !== '' && !value.startsWith('#') && !value.startsWith('//'))
      .map((value) => ({ value, mode, comment: '', enabled: true }));
    const directDomains = [
      ...parseDirectDomains(values.directIncludes, 'include'),
      ...parseDirectDomains(values.directExcludes, 'exclude'),
    ];

    setSubmitting(true);
    try {
      let msg;
      if (isEdit && client) {
        const original = new Set(attachedIds || []);
        const next = new Set(values.inboundIds || []);
        const toAttach = [...next].filter((id) => !original.has(id));
        const toDetach = [...original].filter((id) => !next.has(id));
        msg = await save(clientPayload, {
          isEdit: true,
          email: client.email,
          attach: toAttach,
          detach: toDetach,
          externalLinks: externalLinksPayload,
          subscriptionProfile: profilePayload,
          directIncludes: values.directIncludes,
          directExcludes: values.directExcludes,
        });
      } else {
        msg = await save(
          {
            client: clientPayload,
            clientEnable: !!values.enable,
            inboundIds: values.inboundIds,
            externalLinks: externalLinksPayload,
            subscriptionProfile: profilePayload,
            directDomains,
          },
          {
            isEdit: false,
            email: clientPayload.email as string,
            externalLinks: externalLinksPayload,
            subscriptionProfile: profilePayload,
            directIncludes: values.directIncludes,
            directExcludes: values.directExcludes,
          },
        );
      }
      if (msg?.success) close();
    } finally {
      setSubmitting(false);
    }
  }

  function nextStep() {
    if (currentStep === 0) {
      const value = methods.getValues('email').trim();
      if (!value) {
        messageApi.error(t('pages.clients.email'));
        return;
      }
    }
    if (currentStep === 1 && methods.getValues('inboundIds').length === 0) {
      messageApi.error(t('pages.clients.selectInbound'));
      return;
    }
    if (currentStep === 2) {
      for (const row of methods.getValues('externalLinks')) {
        if (row.kind === 'json' && row.value.trim()) {
          try {
            JSON.parse(row.value);
          } catch (error) {
            messageApi.error(error instanceof Error ? error.message : t('externalJson.invalid'));
            return;
          }
        }
        if (row.kind === 'json_subscription' && row.value.trim() && !row.value.trim().startsWith('https://')) {
          messageApi.error(t('externalJson.httpsRequired'));
          return;
        }
      }
    }
    setCurrentStep((step) => Math.min(step + 1, 3));
  }

  return (
    <>
      {messageContextHolder}
      <Modal
        open={open}
        title={isEdit ? t('pages.clients.editClient') : t('pages.clients.addClient')}
        destroyOnHidden
        width={920}
        zIndex={CLIENT_FORM_MODAL_Z_INDEX}
        style={{ top: 20 }}
        styles={{ body: { maxHeight: 'calc(100vh - 160px)', overflowY: 'auto', overflowX: 'hidden' } }}
        onCancel={close}
        footer={
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            {isEdit && resetTraffic && (
              <Popconfirm
                title={t('pages.inbounds.resetTraffic')}
                description={t('pages.inbounds.resetTrafficContent')}
                okText={t('reset')}
                cancelText={t('cancel')}
                zIndex={CLIENT_IP_LOG_MODAL_Z_INDEX}
                onConfirm={onResetTraffic}
              >
                <Button color="danger" variant="filled" icon={<RetweetOutlined />} loading={resetting}>
                  {t('pages.inbounds.resetTraffic')}
                </Button>
              </Popconfirm>
            )}
            <div style={{ marginInlineStart: 'auto', display: 'flex', gap: 8 }}>
              <Button onClick={close}>{t('cancel')}</Button>
              {currentStep > 0 && (
                <Button onClick={() => setCurrentStep((step) => Math.max(step - 1, 0))}>
                  {t('clientWizard.back')}
                </Button>
              )}
              {currentStep < 3 ? (
                <Button type="primary" onClick={nextStep}>{t('clientWizard.next')}</Button>
              ) : (
                <Button type="primary" loading={submitting} onClick={onSubmit}>
                  {isEdit ? t('save') : t('create')}
                </Button>
              )}
            </div>
          </div>
        }
      >
        <FormProvider {...methods}>
          <Form layout="vertical">
            <Steps
              current={currentStep}
              responsive
              size="small"
              style={{ marginBottom: 24 }}
              onChange={(step) => {
                if (step < currentStep) setCurrentStep(step);
                else if (step === currentStep + 1) nextStep();
              }}
              items={[
                { title: t('clientWizard.client') },
                { title: t('clientWizard.inbounds') },
                { title: t('clientWizard.subscription') },
                { title: t('clientWizard.confirm') },
              ]}
            />
            <Tabs
              activeKey={String(currentStep)}
              tabBarStyle={{ display: 'none' }}
              items={[
                {
                  key: '0',
                  label: t('clientWizard.client'),
                  children: (
                    <>
                      <Row gutter={16}>
                        <Col xs={24} md={12}>
                          <Form.Item label={t('pages.clients.email')} required>
                            <Space.Compact style={{ display: 'flex' }}>
                              <Input
                                value={email}
                                placeholder={t('pages.clients.email')}
                                style={{ flex: 1 }}
                                onChange={(e) => methods.setValue('email', e.target.value)}
                              />
                              {!isEdit && (
                                <Button aria-label={t('regenerate')} icon={<ReloadOutlined />} onClick={() => methods.setValue('email', RandomUtil.randomLowerAndNum(12))} />
                              )}
                            </Space.Compact>
                          </Form.Item>
                        </Col>
                        <Col xs={24} md={6}>
                          <FormField
                            name="totalGB"
                            label={t('pages.clients.totalGB')}
                            tooltip={t('pages.clients.totalGBDesc')}
                            transform={{ output: (v) => Number(v) || 0 }}
                          >
                            <InputNumber min={0} step={1} style={{ width: '100%' }} />
                          </FormField>
                        </Col>
                        <Col xs={24} md={6}>
                          <Form.Item label={t('pages.clients.limitIp')} tooltip={t('pages.clients.limitIpDesc')}>
                            <Tooltip title={limitIpNotice || undefined}>
                              <span style={{ display: 'flex', width: '100%' }}>
                                <Space.Compact style={{ display: 'flex', flex: 1 }}>
                                  <InputNumber value={limitIp} min={0} disabled={limitIpDisabled}
                                    style={{ flex: 1, ...(limitIpDisabled ? { pointerEvents: 'none' } : null) }}
                                    onChange={(v) => methods.setValue('limitIp', Number(v) || 0)} />
                                  {isEdit && (
                                    <Tooltip title={t('pages.clients.ipLog')}>
                                      <Button aria-label={t('pages.clients.ipLog')} icon={<EyeOutlined />} loading={ipsLoading} onClick={openIpsModal}>
                                        {clientIps.length > 0 ? clientIps.length : ''}
                                      </Button>
                                    </Tooltip>
                                  )}
                                </Space.Compact>
                              </span>
                            </Tooltip>
                          </Form.Item>
                        </Col>
                      </Row>

                      <Row gutter={16}>
                        <Col xs={24} md={12}>
                          {delayedStart ? (
                            <FormField
                              name="delayedDays"
                              label={t('pages.clients.expireDays')}
                              transform={{ output: (v) => Number(v) || 0 }}
                            >
                              <InputNumber min={0} style={{ width: '100%' }} />
                            </FormField>
                          ) : (
                            <Form.Item label={t('pages.clients.expiryTime')}>
                              <DateTimePicker
                                value={expiryDayjs}
                                onChange={(d) => methods.setValue('expiryDate', d ? d.valueOf() : 0)}
                              />
                            </Form.Item>
                          )}
                        </Col>
                        <Col xs={12} md={6}>
                          <Form.Item label={t('pages.clients.delayedStart')}>
                            <Switch
                              checked={delayedStart}
                              onChange={(v) => {
                                methods.setValue('delayedStart', v);
                                if (v) methods.setValue('expiryDate', 0);
                                else methods.setValue('delayedDays', 0);
                              }}
                            />
                          </Form.Item>
                        </Col>
                        <Col xs={12} md={6}>
                          <FormField
                            name="reset"
                            label={t('pages.clients.renewDays')}
                            tooltip={t('pages.clients.renewDesc')}
                            transform={{ output: (v) => Number(v) || 0 }}
                          >
                            <InputNumber min={0} style={{ width: '100%' }} />
                          </FormField>
                        </Col>
                      </Row>

                      <Row gutter={16}>
                        <Col xs={24} md={12}>
                          <FormField name="comment" label={t('pages.clients.comment')}>
                            <Input />
                          </FormField>
                        </Col>
                        <Col xs={24} md={12}>
                          <FormField
                            name="group"
                            label={t('pages.clients.group')}
                            tooltip={t('pages.clients.groupDesc')}
                            transform={{ output: (v) => v ?? '' }}
                          >
                            <AutoComplete
                              placeholder={t('pages.clients.groupPlaceholder')}
                              options={groups.map((g) => ({ value: g }))}
                              allowClear
                            />
                          </FormField>
                        </Col>
                      </Row>

                      {(tgBotEnable || showReverseTag) && (
                        <Row gutter={16}>
                          {tgBotEnable && (
                            <Col xs={24} md={12}>
                              <FormField
                                name="tgId"
                                label={t('pages.clients.telegramId')}
                                transform={{ output: (v) => Number(v) || 0 }}
                              >
                                <InputNumber min={0} controls={false}
                                  placeholder={t('pages.clients.telegramIdPlaceholder')} style={{ width: '100%' }} />
                              </FormField>
                            </Col>
                          )}
                          {showReverseTag && (
                            <Col xs={24} md={12}>
                              <FormField name="reverseTag" label={t('pages.clients.reverseTag')}>
                                <Input placeholder={t('pages.clients.reverseTagPlaceholder')} />
                              </FormField>
                            </Col>
                          )}
                        </Row>
                      )}

                      <Form.Item>
                        <Switch aria-label={t('enable')} checked={enable} onChange={(v) => methods.setValue('enable', v)} />
                        <span style={{ marginLeft: 8 }}>{t('enable')}</span>
                      </Form.Item>
                    </>
                  ),
                },
                {
                  key: '1',
                  label: t('clientWizard.inbounds'),
                  children: (
                    <>
                      <Alert
                        type="info"
                        showIcon
                        message={t('clientWizard.inboundHint')}
                        style={{ marginBottom: 16 }}
                      />
                      <Row gutter={12}>
                        <Col xs={24} md={12}>
                          <Form.Item label={t('clientWizard.nodeFilter')}>
                            <Select
                              value={nodeFilter}
                              options={nodeFilterOptions}
                              onChange={setNodeFilter}
                            />
                          </Form.Item>
                        </Col>
                        <Col xs={24} md={12}>
                          <Form.Item label={t('clientWizard.protocolFilter')}>
                            <Select
                              value={protocolFilter}
                              options={protocolFilterOptions}
                              onChange={setProtocolFilter}
                            />
                          </Form.Item>
                        </Col>
                      </Row>
                      <Form.Item label={t('pages.clients.attachedInbounds')} required>
                        <SelectAllClearButtons
                          options={selectableInboundOptions}
                          value={inboundIds}
                          onChange={(v) => methods.setValue('inboundIds', v)}
                        />
                        <Select
                          mode="multiple"
                          value={inboundIds}
                          onChange={(v) => methods.setValue('inboundIds', v)}
                          options={groupedInboundOptions}
                          placeholder={t('pages.clients.selectInbound')}
                          maxTagCount="responsive"
                          placement="bottomLeft"
                          listHeight={260}
                          showSearch={{
                            filterOption: (input, option) => ((option?.label as string) || '').toLowerCase().includes(input.toLowerCase()),
                          }}
                        />
                      </Form.Item>
                      <Typography.Paragraph type="secondary">
                        {t('clientWizard.selectedCount', { count: inboundIds.length })}
                      </Typography.Paragraph>
                      <Divider>{t('pages.clients.tabCredentials')}</Divider>
                      <Form.Item label={t('pages.clients.uuid')}>
                        <Space.Compact style={{ display: 'flex' }}>
                          <Input value={uuid} style={{ flex: 1 }} onChange={(e) => methods.setValue('uuid', e.target.value)} />
                          <Button aria-label={t('regenerate')} icon={<ReloadOutlined />} onClick={() => methods.setValue('uuid', RandomUtil.randomUUID())} />
                        </Space.Compact>
                      </Form.Item>

                      <Form.Item label={t('pages.clients.password')} tooltip={t('pages.clients.passwordDesc')}>
                        <Space.Compact style={{ display: 'flex' }}>
                          <Input value={password} style={{ flex: 1 }} onChange={(e) => methods.setValue('password', e.target.value)} />
                          <Button aria-label={t('regenerate')} icon={<ReloadOutlined />} onClick={regeneratePassword} />
                        </Space.Compact>
                      </Form.Item>

                      <Form.Item label={t('pages.clients.subId')}>
                        <Space.Compact style={{ display: 'flex' }}>
                          <Input value={subId} style={{ flex: 1 }} onChange={(e) => methods.setValue('subId', e.target.value)} />
                          <Button aria-label={t('regenerate')} icon={<ReloadOutlined />} onClick={() => methods.setValue('subId', RandomUtil.randomLowerAndNum(16))} />
                        </Space.Compact>
                      </Form.Item>

                      <Form.Item label={t('pages.clients.hysteriaAuth')} tooltip={t('pages.clients.hysteriaAuthDesc')}>
                        <Space.Compact style={{ display: 'flex' }}>
                          <Input value={auth} style={{ flex: 1 }} onChange={(e) => methods.setValue('auth', e.target.value)} />
                          <Button aria-label={t('regenerate')} icon={<ReloadOutlined />} onClick={() => methods.setValue('auth', RandomUtil.randomLowerAndNum(16))} />
                        </Space.Compact>
                      </Form.Item>

                      {showFlow && (
                        <FormField name="flow" label={t('pages.clients.flow')}>
                          <Select
                            options={[
                              { value: '', label: t('none') },
                              ...FLOW_OPTIONS.map((k) => ({ value: k, label: k })),
                            ]}
                          />
                        </FormField>
                      )}
                      {showSecurity && (
                        <FormField name="security" label={t('pages.clients.vmessSecurity')}>
                          <Select
                            options={VMESS_SECURITY_OPTIONS.map((k) => ({ value: k, label: k }))}
                          />
                        </FormField>
                      )}
                      {showWireguard && (
                        <>
                          <Form.Item label={t('pages.clients.wireguardPrivateKey')}>
                            <Space.Compact style={{ display: 'flex' }}>
                              <Input
                                value={wgPrivateKey}
                                style={{ flex: 1 }}
                                onChange={(e) => {
                                  const priv = e.target.value;
                                  methods.setValue('wgPrivateKey', priv);
                                  methods.setValue('wgPublicKey', priv ? Wireguard.generateKeypair(priv).publicKey : '');
                                }}
                              />
                              <Button aria-label={t('regenerate')} icon={<ReloadOutlined />} onClick={regenerateWireguardKeys} />
                            </Space.Compact>
                          </Form.Item>
                          <FormField name="wgPublicKey" label={t('pages.clients.wireguardPublicKey')}>
                            <Input disabled />
                          </FormField>
                          <FormField name="wgPreSharedKey" label={t('pages.clients.wireguardPreSharedKey')}>
                            <Input />
                          </FormField>
                          <FormField
                            name="wgAllowedIPs"
                            label={t('pages.clients.wireguardAllowedIPs')}
                            extra={t('pages.clients.wireguardAllowedIPsHint')}
                          >
                            <Input placeholder="10.0.0.2/32" />
                          </FormField>
                        </>
                      )}
                      {showMtproto && (
                        <>
                          <Form.Item label={t('pages.clients.mtprotoSecret')} extra={t('pages.clients.mtprotoSecretHint')}>
                            <Space.Compact style={{ display: 'flex' }}>
                              <Input value={secret} style={{ flex: 1 }} onChange={(e) => methods.setValue('secret', e.target.value)} />
                              <Button aria-label={t('regenerate')} icon={<ReloadOutlined />} onClick={regenerateMtprotoSecret} />
                            </Space.Compact>
                          </Form.Item>
                          <FormField
                            name="adTag"
                            label={t('pages.clients.mtprotoAdTag')}
                            extra={t('pages.clients.mtprotoAdTagHint')}
                          >
                            <Input
                              allowClear
                              placeholder="0123456789abcdef0123456789abcdef"
                            />
                          </FormField>
                        </>
                      )}
                    </>
                  ),
                },
                {
                  key: '2',
                  label: t('clientWizard.subscription'),
                  children: (
                    <>
                      <Typography.Paragraph type="secondary" style={{ marginTop: 4 }}>
                        {t('pages.clients.linksHint')}
                      </Typography.Paragraph>

                      <Button type="primary" icon={<PlusOutlined />} onClick={() => addExternalLinkRow('link')}>
                        {t('pages.clients.addExternalLink')}
                      </Button>
                      <div style={{ marginTop: 12, marginBottom: 24 }}>
                        {linkRows.length === 0 ? (
                          <Typography.Text type="secondary">{t('pages.clients.noExternalLinks')}</Typography.Text>
                        ) : linkRows.map(({ field, index }) => (
                          <div key={field.id} style={{ display: 'flex', gap: 8, marginBottom: 8 }}>
                            <FormField name={`externalLinks.${index}.value`} noStyle>
                              <Input
                                style={{ flex: 1 }}
                                aria-label="vless:// · vmess:// · trojan:// · ss:// · hysteria2:// · wireguard://"
                                placeholder="vless:// · vmess:// · trojan:// · ss:// · hysteria2:// · wireguard://"
                              />
                            </FormField>
                            <Tooltip title={t('delete')}>
                              <Button aria-label={t('delete')} danger icon={<DeleteOutlined />} onClick={() => removeExternalLink(index)} />
                            </Tooltip>
                          </div>
                        ))}
                      </div>

                      <Button type="primary" icon={<PlusOutlined />} onClick={() => addExternalLinkRow('subscription')}>
                        {t('pages.clients.addExternalSubscription')}
                      </Button>
                      <div style={{ marginTop: 12 }}>
                        {subscriptionRows.length === 0 ? (
                          <Typography.Text type="secondary">{t('pages.clients.noExternalSubscriptions')}</Typography.Text>
                        ) : subscriptionRows.map(({ field, index }) => (
                          <div key={field.id} style={{ display: 'flex', gap: 8, marginBottom: 8 }}>
                            <FormField name={`externalLinks.${index}.value`} noStyle>
                              <Input
                                style={{ flex: 1 }}
                                aria-label="https://provider.example/sub/…"
                                placeholder="https://provider.example/sub/…"
                              />
                            </FormField>
                            <Tooltip title={t('delete')}>
                              <Button aria-label={t('delete')} danger icon={<DeleteOutlined />} onClick={() => removeExternalLink(index)} />
                            </Tooltip>
                          </div>
                        ))}
                      </div>

                      <Divider>{t('externalJson.section')}</Divider>
                      <Space wrap>
                        <Button icon={<PlusOutlined />} onClick={() => addExternalLinkRow('json')}>
                          {t('externalJson.addManual')}
                        </Button>
                        <Button icon={<PlusOutlined />} onClick={() => addExternalLinkRow('json_subscription')}>
                          {t('externalJson.addRemote')}
                        </Button>
                      </Space>

                      {manualJsonRows.map(({ field, index }) => (
                        <Card
                          key={field.id}
                          size="small"
                          title={t('externalJson.manual')}
                          style={{ marginTop: 16 }}
                          extra={(
                            <Space.Compact>
                              <Button
                                type="text"
                                icon={<ArrowUpOutlined />}
                                disabled={index === 0}
                                aria-label={t('externalJson.moveUp')}
                                onClick={() => moveExternalLink(index, index - 1)}
                              />
                              <Button
                                type="text"
                                icon={<ArrowDownOutlined />}
                                disabled={index === externalLinkFields.length - 1}
                                aria-label={t('externalJson.moveDown')}
                                onClick={() => moveExternalLink(index, index + 1)}
                              />
                              <Popconfirm
                                title={t('externalJson.deleteConfirm')}
                                onConfirm={() => removeExternalLink(index)}
                              >
                                <Button danger type="text" icon={<DeleteOutlined />} aria-label={t('delete')} />
                              </Popconfirm>
                            </Space.Compact>
                          )}
                        >
                          <Row gutter={12}>
                            <Col xs={24} md={10}>
                              <FormField name={`externalLinks.${index}.remark`} label={t('externalJson.name')}>
                                <Input />
                              </FormField>
                            </Col>
                            <Col xs={24} md={10}>
                              <FormField name={`externalLinks.${index}.comment`} label={t('externalJson.comment')}>
                                <Input />
                              </FormField>
                            </Col>
                            <Col xs={12} md={2}>
                              <Form.Item label={t('enable')}>
                                <Switch
                                  checked={externalLinks[index]?.enabled !== false}
                                  onChange={(checked) => methods.setValue(`externalLinks.${index}.enabled`, checked)}
                                />
                              </Form.Item>
                            </Col>
                            <Col xs={12} md={2}>
                              <FormField
                                name={`externalLinks.${index}.priority`}
                                label={t('externalJson.priority')}
                                transform={{ output: (value) => Number(value) || 0 }}
                              >
                                <InputNumber style={{ width: '100%' }} />
                              </FormField>
                            </Col>
                          </Row>
                          <JsonEditor
                            value={externalLinks[index]?.value || ''}
                            minHeight="220px"
                            maxHeight="420px"
                            onChange={(value) => methods.setValue(`externalLinks.${index}.value`, value)}
                          />
                          <Space wrap style={{ marginTop: 12 }}>
                            <Button onClick={() => validateManualJSON(index)}>{t('externalJson.validate')}</Button>
                            <Button onClick={() => validateManualJSON(index, true)}>{t('externalJson.format')}</Button>
                            {externalLinks[index]?.updatedAt ? (
                              <Typography.Text type="secondary">
                                {t('externalJson.updated')}: {dayjs(externalLinks[index].updatedAt).format('YYYY-MM-DD HH:mm')}
                              </Typography.Text>
                            ) : null}
                          </Space>
                        </Card>
                      ))}

                      {remoteJsonRows.map(({ field, index }) => (
                        <Card
                          key={field.id}
                          size="small"
                          title={t('externalJson.remote')}
                          style={{ marginTop: 16 }}
                          extra={(
                            <Space.Compact>
                              <Button
                                type="text"
                                icon={<ArrowUpOutlined />}
                                disabled={index === 0}
                                aria-label={t('externalJson.moveUp')}
                                onClick={() => moveExternalLink(index, index - 1)}
                              />
                              <Button
                                type="text"
                                icon={<ArrowDownOutlined />}
                                disabled={index === externalLinkFields.length - 1}
                                aria-label={t('externalJson.moveDown')}
                                onClick={() => moveExternalLink(index, index + 1)}
                              />
                              <Popconfirm
                                title={t('externalJson.deleteConfirm')}
                                onConfirm={() => removeExternalLink(index)}
                              >
                                <Button danger type="text" icon={<DeleteOutlined />} aria-label={t('delete')} />
                              </Popconfirm>
                            </Space.Compact>
                          )}
                        >
                          <Row gutter={12}>
                            <Col xs={24} md={12}>
                              <FormField name={`externalLinks.${index}.remark`} label={t('externalJson.name')}>
                                <Input />
                              </FormField>
                            </Col>
                            <Col xs={24} md={12}>
                              <FormField name={`externalLinks.${index}.comment`} label={t('externalJson.comment')}>
                                <Input />
                              </FormField>
                            </Col>
                          </Row>
                          <FormField name={`externalLinks.${index}.value`} label={t('externalJson.httpsUrl')}>
                            <Input placeholder="https://provider.example/config.json" />
                          </FormField>
                          <Row gutter={12}>
                            <Col xs={12} md={4}>
                              <Form.Item label={t('enable')}>
                                <Switch
                                  checked={externalLinks[index]?.enabled !== false}
                                  onChange={(checked) => methods.setValue(`externalLinks.${index}.enabled`, checked)}
                                />
                              </Form.Item>
                            </Col>
                            <Col xs={12} md={4}>
                              <FormField
                                name={`externalLinks.${index}.priority`}
                                label={t('externalJson.priority')}
                                transform={{ output: (value) => Number(value) || 0 }}
                              >
                                <InputNumber style={{ width: '100%' }} />
                              </FormField>
                            </Col>
                            <Col xs={12} md={4}>
                              <FormField
                                name={`externalLinks.${index}.updateIntervalMinutes`}
                                label={t('externalJson.intervalMinutes')}
                                transform={{ output: (value) => Number(value) || 60 }}
                              >
                                <InputNumber min={1} max={10080} style={{ width: '100%' }} />
                              </FormField>
                            </Col>
                            <Col xs={12} md={4}>
                              <FormField
                                name={`externalLinks.${index}.timeoutSeconds`}
                                label={t('externalJson.timeoutSeconds')}
                                transform={{ output: (value) => Number(value) || 8 }}
                              >
                                <InputNumber min={1} max={60} style={{ width: '100%' }} />
                              </FormField>
                            </Col>
                            <Col xs={12} md={4}>
                              <FormField
                                name={`externalLinks.${index}.maxResponseBytes`}
                                label={t('externalJson.maxBytes')}
                                transform={{ output: (value) => Number(value) || 2097152 }}
                              >
                                <InputNumber min={1024} max={16777216} style={{ width: '100%' }} />
                              </FormField>
                            </Col>
                            <Col xs={12} md={4}>
                              <FormField
                                name={`externalLinks.${index}.maxRedirects`}
                                label={t('externalJson.redirects')}
                                transform={{ output: (value) => Number(value) || 0 }}
                              >
                                <InputNumber min={0} max={5} style={{ width: '100%' }} />
                              </FormField>
                            </Col>
                          </Row>
                          {externalLinks[index]?.lastError ? (
                            <Alert type="error" showIcon message={externalLinks[index].lastError} style={{ marginBottom: 12 }} />
                          ) : null}
                          <Space wrap>
                            <Button
                              icon={<ReloadOutlined />}
                              disabled={!externalLinks[index]?.id}
                              onClick={() => refreshRemoteSource(index)}
                            >
                              {t('externalJson.refresh')}
                            </Button>
                            <Typography.Text type="secondary">
                              {t('externalJson.httpStatus')}: {externalLinks[index]?.lastHttpStatus || '-'}
                            </Typography.Text>
                            <Typography.Text type="secondary">
                              {t('externalJson.lastSuccess')}: {externalLinks[index]?.lastSuccessAt
                                ? dayjs(externalLinks[index].lastSuccessAt).format('YYYY-MM-DD HH:mm')
                                : '-'}
                            </Typography.Text>
                            <Typography.Text type="secondary">
                              {t('externalJson.lastAttempt')}: {externalLinks[index]?.lastAttemptAt
                                ? dayjs(externalLinks[index].lastAttemptAt).format('YYYY-MM-DD HH:mm')
                                : '-'}
                            </Typography.Text>
                          </Space>
                        </Card>
                      ))}

                      <Divider>{t('happ.settingsEnableTitle')}</Divider>
                      <Row gutter={12}>
                        <Col xs={12} md={4}>
                          <Form.Item label={t('enable')}>
                            <Switch
                              checked={subscriptionProfile.enabled}
                              onChange={(checked) => methods.setValue('subscriptionProfile.enabled', checked)}
                            />
                          </Form.Item>
                        </Col>
                        <Col xs={24} md={8}>
                          <FormField name="subscriptionProfile.displayName" label={t('clientWizard.subscriptionName')}>
                            <Input placeholder={email} />
                          </FormField>
                        </Col>
                        <Col xs={12} md={4}>
                          <FormField name="subscriptionProfile.language" label={t('clientWizard.language')}>
                            <Select options={[
                              { value: 'en', label: t('clientWizard.languageEnglish') },
                              { value: 'ru', label: t('clientWizard.languageRussian') },
                            ]} />
                          </FormField>
                        </Col>
                        <Col xs={12} md={4}>
                          <FormField
                            name="subscriptionProfile.updateInterval"
                            label={t('externalJson.intervalMinutes')}
                            transform={{ output: (value) => Number(value) || 60 }}
                          >
                            <InputNumber min={1} max={10080} style={{ width: '100%' }} />
                          </FormField>
                        </Col>
                      </Row>
                      <Row gutter={12}>
                        <Col xs={24} md={12}>
                          <FormField name="subscriptionProfile.title" label={t('clientWizard.subscriptionTitle')}>
                            <Input placeholder={email} />
                          </FormField>
                        </Col>
                        <Col xs={24} md={12}>
                          <Form.Item label={t('clientWizard.linkExpiresAt')}>
                            <DateTimePicker
                              value={subscriptionProfile.linkExpiresAt > 0
                                ? dayjs(subscriptionProfile.linkExpiresAt)
                                : null}
                              onChange={(value) => methods.setValue(
                                'subscriptionProfile.linkExpiresAt',
                                value ? value.valueOf() : 0,
                              )}
                            />
                          </Form.Item>
                        </Col>
                      </Row>

                      <Button
                        type={subscriptionProfile.autoSelectEnabled ? 'primary' : 'default'}
                        icon={<ReloadOutlined />}
                        onClick={() => {
                          const next = !subscriptionProfile.autoSelectEnabled;
                          methods.setValue('subscriptionProfile.autoSelectEnabled', next);
                          if (next && !methods.getValues('subscriptionProfile.autoSelectName').trim()) {
                            methods.setValue('subscriptionProfile.autoSelectName', t('clientWizard.defaultAutoSelectName'));
                          }
                        }}
                      >
                        {t('clientWizard.createAutoSelect')}
                      </Button>
                      {subscriptionProfile.autoSelectEnabled && (
                        <Card size="small" style={{ marginTop: 12 }}>
                          <Alert
                            type="info"
                            showIcon
                            message={t('clientWizard.autoSelectClientPing')}
                            style={{ marginBottom: 12 }}
                          />
                          <Row gutter={12}>
                            <Col xs={24} md={12}>
                              <FormField name="subscriptionProfile.autoSelectName" label={t('clientWizard.groupName')}>
                                <Input />
                              </FormField>
                            </Col>
                            <Col xs={24} md={12}>
                              <FormField name="subscriptionProfile.probeUrl" label={t('clientWizard.probeUrl')}>
                                <Input />
                              </FormField>
                            </Col>
                            <Col xs={12} md={6}>
                              <FormField
                                name="subscriptionProfile.probeTimeoutSeconds"
                                label={t('externalJson.timeoutSeconds')}
                                transform={{ output: (value) => Number(value) || 5 }}
                              >
                                <InputNumber min={1} max={30} style={{ width: '100%' }} />
                              </FormField>
                            </Col>
                            <Col xs={12} md={6}>
                              <FormField
                                name="subscriptionProfile.probeIntervalSeconds"
                                label={t('clientWizard.probeInterval')}
                                transform={{ output: (value) => Number(value) || 300 }}
                              >
                                <InputNumber min={30} max={86400} style={{ width: '100%' }} />
                              </FormField>
                            </Col>
                            <Col xs={12} md={6}>
                              <FormField
                                name="subscriptionProfile.switchThresholdMs"
                                label={t('clientWizard.switchThreshold')}
                                transform={{ output: (value) => Number(value) || 0 }}
                              >
                                <InputNumber min={0} max={60000} style={{ width: '100%' }} />
                              </FormField>
                            </Col>
                            <Col xs={12} md={6}>
                              <FormField
                                name="subscriptionProfile.debounceSeconds"
                                label={t('clientWizard.debounce')}
                                transform={{ output: (value) => Number(value) || 0 }}
                              >
                                <InputNumber min={0} max={86400} style={{ width: '100%' }} />
                              </FormField>
                            </Col>
                          </Row>
                          <Form.Item label={t('clientWizard.candidates')}>
                            <Select
                              mode="multiple"
                              value={subscriptionProfile.autoSelectCandidates}
                              options={autoCandidateOptions}
                              placeholder={t('clientWizard.allCandidates')}
                              onChange={(value) => methods.setValue('subscriptionProfile.autoSelectCandidates', value)}
                            />
                          </Form.Item>
                          <Form.Item label={t('clientWizard.fallback')}>
                            <Select
                              allowClear
                              value={subscriptionProfile.fallbackTag || undefined}
                              options={autoCandidateOptions}
                              onChange={(value) => methods.setValue('subscriptionProfile.fallbackTag', value || '')}
                            />
                          </Form.Item>
                        </Card>
                      )}

                      <Divider>{t('directDomains.title')}</Divider>
                      <Row gutter={12}>
                        <Col xs={24} md={12}>
                          <FormField name="directIncludes" label={t('clientWizard.additionalDirectDomains')}>
                            <Input.TextArea autoSize={{ minRows: 5, maxRows: 10 }} placeholder={t('directDomains.placeholder')} />
                          </FormField>
                        </Col>
                        <Col xs={24} md={12}>
                          <FormField name="directExcludes" label={t('clientWizard.directDomainExclusions')}>
                            <Input.TextArea autoSize={{ minRows: 5, maxRows: 10 }} placeholder={t('directDomains.placeholder')} />
                          </FormField>
                        </Col>
                      </Row>
                    </>
                  ),
                },
                {
                  key: '3',
                  label: t('clientWizard.confirm'),
                  children: (
                    <>
                      <Alert type="info" showIcon message={t('clientWizard.reviewHint')} style={{ marginBottom: 16 }} />
                      <Card size="small" title={t('clientWizard.client')}>
                        <Typography.Paragraph><strong>{t('pages.clients.email')}:</strong> {email}</Typography.Paragraph>
                        <Typography.Paragraph><strong>{t('pages.clients.subId')}:</strong> {subId}</Typography.Paragraph>
                        <Typography.Paragraph><strong>{t('pages.clients.limitIp')}:</strong> {limitIp || t('clientWizard.unlimited')}</Typography.Paragraph>
                        <Typography.Paragraph><strong>{t('enable')}:</strong> {enable ? t('clientWizard.yes') : t('clientWizard.no')}</Typography.Paragraph>
                      </Card>
                      <Card size="small" title={t('clientWizard.inbounds')} style={{ marginTop: 12 }}>
                        <Space wrap>
                          {inboundOptions
                            .filter((option) => inboundIds.includes(option.value))
                            .map((option) => <Tag key={option.value}>{option.label}</Tag>)}
                        </Space>
                      </Card>
                      <Card size="small" title={t('clientWizard.subscription')} style={{ marginTop: 12 }}>
                        <Typography.Paragraph>
                          <strong>{t('happ.settingsEnableTitle')}:</strong> {subscriptionProfile.enabled ? t('clientWizard.yes') : t('clientWizard.no')}
                        </Typography.Paragraph>
                        <Typography.Paragraph>
                          <strong>{t('externalJson.section')}:</strong> {externalLinks.filter((row) => row.value.trim()).length}
                        </Typography.Paragraph>
                        <Typography.Paragraph>
                          <strong>{t('clientWizard.createAutoSelect')}:</strong> {subscriptionProfile.autoSelectEnabled ? t('clientWizard.yes') : t('clientWizard.no')}
                        </Typography.Paragraph>
                        <Typography.Paragraph>
                          <strong>{t('directDomains.title')}:</strong>{' '}
                          {directIncludes.split(/[\s,]+/).filter(Boolean).length} / {directExcludes.split(/[\s,]+/).filter(Boolean).length}
                        </Typography.Paragraph>
                      </Card>
                      {subscriptionProfile.autoSelectEnabled && autoCandidateOptions.length === 0 ? (
                        <Alert type="warning" showIcon message={t('clientWizard.noAutoCandidates')} style={{ marginTop: 12 }} />
                      ) : null}
                    </>
                  ),
                },
              ]}
            />
          </Form>
        </FormProvider>
      </Modal>

      <Modal
        open={ipsModalOpen}
        title={`${t('pages.clients.ipLog')}${client?.email ? ` — ${client.email}` : ''}`}
        width={440}
        zIndex={CLIENT_IP_LOG_MODAL_Z_INDEX}
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
