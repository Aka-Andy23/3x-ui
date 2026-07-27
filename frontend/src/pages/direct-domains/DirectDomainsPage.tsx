import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Card,
  Col,
  ConfigProvider,
  Input,
  Layout,
  Modal,
  Popconfirm,
  Row,
  Space,
  Spin,
  Statistic,
  Switch,
  Table,
  Tag,
  message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  DeleteOutlined,
  DownloadOutlined,
  EyeOutlined,
  ImportOutlined,
  ReloadOutlined,
  SearchOutlined,
} from '@ant-design/icons';

import AppSidebar from '@/layouts/AppSidebar';
import { JsonEditor } from '@/components/form';
import { HttpUtil } from '@/utils';
import { useTheme } from '@/hooks/useTheme';

interface DirectDomain {
  id: number;
  clientId: number;
  mode: 'include' | 'exclude';
  domain: string;
  displayDomain: string;
  comment: string;
  enabled: boolean;
  createdAt: number;
  updatedAt: number;
}

interface ApiMsg<T = unknown> {
  success?: boolean;
  msg?: string;
  obj?: T;
}

interface ImportResult {
  imported?: number;
  skipped?: number;
  invalid?: string[];
}

export default function DirectDomainsPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const [messageApi, messageContextHolder] = message.useMessage();
  const [rows, setRows] = useState<DirectDomain[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [search, setSearch] = useState('');
  const [raw, setRaw] = useState('');
  const [comment, setComment] = useState('');
  const [invalid, setInvalid] = useState<string[]>([]);
  const [previewOpen, setPreviewOpen] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const msg = await HttpUtil.get('/panel/api/directDomains/list', undefined, { silent: true }) as ApiMsg<DirectDomain[]>;
      if (!msg?.success) {
        messageApi.error(msg?.msg || t('somethingWentWrong'));
        return;
      }
      setRows(Array.isArray(msg.obj) ? msg.obj : []);
    } finally {
      setLoading(false);
    }
  }, [messageApi, t]);

  useEffect(() => {
    void load();
  }, [load]);

  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase();
    if (!query) return rows;
    return rows.filter((row) =>
      row.domain.toLowerCase().includes(query)
      || row.displayDomain.toLowerCase().includes(query)
      || row.comment.toLowerCase().includes(query));
  }, [rows, search]);

  const routingPreview = useMemo(() => JSON.stringify({
    type: 'field',
    domain: rows.filter((row) => row.enabled).map((row) => `domain:${row.domain}`),
    outboundTag: 'direct',
  }, null, 2), [rows]);

  async function importDomains() {
    if (!raw.trim()) return;
    setSaving(true);
    try {
      const msg = await HttpUtil.post('/panel/api/directDomains/import', {
        raw,
        mode: 'include',
        comment,
      }, { headers: { 'Content-Type': 'application/json' } }) as ApiMsg<ImportResult>;
      if (!msg?.success) {
        messageApi.error(msg?.msg || t('somethingWentWrong'));
        return;
      }
      setInvalid(msg.obj?.invalid ?? []);
      setRaw('');
      messageApi.success(t('directDomains.imported', { count: msg.obj?.imported ?? 0 }));
      await load();
    } finally {
      setSaving(false);
    }
  }

  async function setEnabled(row: DirectDomain, enabled: boolean) {
    const msg = await HttpUtil.post('/panel/api/directDomains/upsert', {
      domain: {
        id: row.id,
        value: row.domain,
        mode: row.mode,
        comment: row.comment,
        enabled,
      },
    }, { headers: { 'Content-Type': 'application/json' } }) as ApiMsg<DirectDomain>;
    if (!msg?.success) {
      messageApi.error(msg?.msg || t('somethingWentWrong'));
      return;
    }
    await load();
  }

  async function remove(row: DirectDomain) {
    const msg = await HttpUtil.post(`/panel/api/directDomains/del/${row.id}`) as ApiMsg;
    if (!msg?.success) {
      messageApi.error(msg?.msg || t('somethingWentWrong'));
      return;
    }
    await load();
  }

  function exportDomains() {
    const content = rows.map((row) => `${row.domain}${row.comment ? ` # ${row.comment}` : ''}`).join('\n');
    const blob = new Blob([content], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = 'direct-domains.txt';
    anchor.click();
    URL.revokeObjectURL(url);
  }

  const columns: ColumnsType<DirectDomain> = [
    {
      title: t('domainName'),
      dataIndex: 'domain',
      render: (_value, row) => (
        <Space direction="vertical" size={0}>
          <span>{row.displayDomain || row.domain}</span>
          {row.displayDomain !== row.domain ? <span className="hint">{row.domain}</span> : null}
        </Space>
      ),
    },
    {
      title: t('directDomains.comment'),
      dataIndex: 'comment',
      responsive: ['md'],
      render: (value: string) => value || '—',
    },
    {
      title: t('status'),
      key: 'enabled',
      width: 100,
      render: (_value, row) => <Switch checked={row.enabled} onChange={(value) => setEnabled(row, value)} />,
    },
    {
      title: t('actions'),
      key: 'actions',
      width: 84,
      render: (_value, row) => (
        <Popconfirm
          title={t('directDomains.deleteConfirm')}
          okText={t('delete')}
          cancelText={t('cancel')}
          onConfirm={() => remove(row)}
        >
          <Button danger icon={<DeleteOutlined />} aria-label={t('delete')} />
        </Popconfirm>
      ),
    },
  ];

  const pageClass = ['direct-domains-page', isDark ? 'is-dark' : '', isUltra ? 'is-ultra' : ''].filter(Boolean).join(' ');

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {messageContextHolder}
      <Layout className={pageClass}>
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <Spin spinning={loading} description={t('loading')}>
              <Row gutter={[16, 16]}>
                <Col xs={24} md={8}>
                  <Card size="small">
                    <Statistic title={t('directDomains.title')} value={rows.length} />
                  </Card>
                </Col>
                <Col xs={24} md={16}>
                  <Card size="small">
                    <Space wrap>
                      <Button icon={<ReloadOutlined />} loading={loading} onClick={load}>{t('refresh')}</Button>
                      <Button icon={<EyeOutlined />} onClick={() => setPreviewOpen(true)}>{t('directDomains.preview')}</Button>
                      <Button icon={<DownloadOutlined />} disabled={rows.length === 0} onClick={exportDomains}>{t('export')}</Button>
                    </Space>
                  </Card>
                </Col>
                <Col span={24}>
                  <Card title={t('directDomains.bulkImport')}>
                    <Input.TextArea
                      value={raw}
                      rows={7}
                      placeholder={t('directDomains.placeholder')}
                      onChange={(event) => setRaw(event.target.value)}
                    />
                    <Input
                      value={comment}
                      style={{ marginTop: 12 }}
                      placeholder={t('directDomains.comment')}
                      onChange={(event) => setComment(event.target.value)}
                    />
                    <Button
                      type="primary"
                      icon={<ImportOutlined />}
                      loading={saving}
                      disabled={!raw.trim()}
                      style={{ marginTop: 12 }}
                      onClick={importDomains}
                    >
                      {t('import')}
                    </Button>
                    {invalid.length > 0 ? (
                      <Alert
                        type="warning"
                        showIcon
                        style={{ marginTop: 12 }}
                        title={t('directDomains.invalid', { count: invalid.length })}
                        description={invalid.join(', ')}
                      />
                    ) : null}
                  </Card>
                </Col>
                <Col span={24}>
                  <Card>
                    <Input
                      value={search}
                      allowClear
                      prefix={<SearchOutlined />}
                      placeholder={t('search')}
                      style={{ marginBottom: 12 }}
                      onChange={(event) => setSearch(event.target.value)}
                    />
                    <Table
                      rowKey="id"
                      columns={columns}
                      dataSource={filtered}
                      pagination={{ pageSize: 25, hideOnSinglePage: true }}
                      scroll={{ x: 620 }}
                      locale={{ emptyText: t('directDomains.empty') }}
                    />
                    <Tag>{t('directDomains.count', { count: filtered.length })}</Tag>
                  </Card>
                </Col>
              </Row>
            </Spin>
          </Layout.Content>
        </Layout>
      </Layout>
      <Modal
        open={previewOpen}
        title={t('directDomains.preview')}
        width={760}
        footer={<Button type="primary" onClick={() => setPreviewOpen(false)}>{t('close')}</Button>}
        onCancel={() => setPreviewOpen(false)}
      >
        <JsonEditor value={routingPreview} readOnly minHeight="300px" maxHeight="60vh" />
      </Modal>
    </ConfigProvider>
  );
}
