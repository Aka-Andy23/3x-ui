import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Modal,
  Space,
  Table,
  Tag,
  Typography,
  Upload,
  message,
} from 'antd';
import {
  DownloadOutlined,
  FileSearchOutlined,
  UploadOutlined,
} from '@ant-design/icons';

import { JsonEditor } from '@/components/form';
import {
  CLIENT_IMPORT_MAX_BYTES,
  parseClientImport,
  type ClientImportErrorCode,
} from '@/lib/clients/import-preview';

interface ImportSkipped {
  email?: string;
  reason?: string;
}

interface ImportResult {
  created?: number;
  skipped?: ImportSkipped[];
}

interface ApiMsg {
  success?: boolean;
  msg?: string;
  obj?: ImportResult | null;
}

interface ClientImportModalProps {
  open: boolean;
  importClients: (data: string) => Promise<ApiMsg>;
  onOpenChange: (open: boolean) => void;
  onSaved?: () => void;
}

const EMPTY_DOCUMENT = '[\n  {\n    "client": {\n      "email": ""\n    },\n    "inboundIds": []\n  }\n]';

export default function ClientImportModal({
  open,
  importClients,
  onOpenChange,
  onSaved,
}: ClientImportModalProps) {
  const { t } = useTranslation();
  const [messageApi, messageContextHolder] = message.useMessage();
  const [value, setValue] = useState(EMPTY_DOCUMENT);
  const [saving, setSaving] = useState(false);
  const [result, setResult] = useState<ImportResult | null>(null);

  useEffect(() => {
    if (!open) return;
    setValue(EMPTY_DOCUMENT);
    setResult(null);
  }, [open]);

  const preview = useMemo(() => parseClientImport(value), [value]);
  const localInvalid = preview.rows.some((row) => row.errors.length > 0);
  const topError = preview.parseError
    || (preview.errorCode ? t(`clientImport.errors.${preview.errorCode}`) : '');
  const skippedByEmail = useMemo(
    () => new Map((result?.skipped || []).map((row) => [(row.email || '').toLowerCase(), row.reason || ''])),
    [result],
  );

  function errorText(errors: ClientImportErrorCode[]): string {
    return errors.map((code) => t(`clientImport.errors.${code}`)).join('; ');
  }

  function downloadReport() {
    const local = preview.rows
      .filter((row) => row.errors.length > 0)
      .map((row) => ({ row: row.index, email: row.email, reason: errorText(row.errors) }));
    const report = {
      generatedAt: new Date().toISOString(),
      inputRows: preview.rows.length,
      created: result?.created || 0,
      errors: [
        ...local,
        ...(result?.skipped || []).map((row) => ({
          email: row.email || '',
          reason: row.reason || '',
        })),
      ],
    };
    const blob = new Blob([JSON.stringify(report, null, 2)], { type: 'application/json;charset=utf-8' });
    const href = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = href;
    anchor.download = 'client-import-report.json';
    anchor.click();
    URL.revokeObjectURL(href);
  }

  async function submit() {
    if (topError || localInvalid || preview.rows.length === 0) {
      messageApi.error(topError || t('clientImport.fixErrors'));
      return;
    }
    setSaving(true);
    try {
      const response = await importClients(value);
      if (!response?.success) {
        messageApi.error(response?.msg || t('somethingWentWrong'));
        return;
      }
      const next = response.obj || {};
      setResult(next);
      if ((next.skipped || []).length > 0) {
        messageApi.warning(t('clientImport.partial', {
          created: next.created || 0,
          failed: next.skipped?.length || 0,
        }));
      } else {
        messageApi.success(t('pages.clients.toasts.imported', { count: next.created || 0 }));
      }
      onSaved?.();
    } finally {
      setSaving(false);
    }
  }

  return (
    <>
      {messageContextHolder}
      <Modal
        open={open}
        width={920}
        title={t('pages.clients.importClients')}
        okText={t('pages.clients.import')}
        cancelText={t('close')}
        confirmLoading={saving}
        okButtonProps={{ disabled: !!topError || localInvalid || preview.rows.length === 0 }}
        onOk={submit}
        onCancel={() => onOpenChange(false)}
      >
        <Space wrap style={{ marginBottom: 12 }}>
          <Upload
            accept=".json,application/json"
            showUploadList={false}
            beforeUpload={(file) => {
              if (file.size > CLIENT_IMPORT_MAX_BYTES) {
                messageApi.error(t('clientImport.errors.tooLarge'));
                return Upload.LIST_IGNORE;
              }
              void file.text().then(setValue);
              setResult(null);
              return false;
            }}
          >
            <Button icon={<UploadOutlined />}>{t('clientImport.loadFile')}</Button>
          </Upload>
          <Button
            icon={<DownloadOutlined />}
            disabled={!localInvalid && !(result?.skipped || []).length}
            onClick={downloadReport}
          >
            {t('clientImport.downloadReport')}
          </Button>
          <Typography.Text type="secondary">
            {t('clientImport.formatHint')}
          </Typography.Text>
        </Space>

        <JsonEditor
          value={value}
          minHeight="220px"
          maxHeight="360px"
          onChange={(next) => {
            setValue(next);
            setResult(null);
          }}
        />

        {topError ? <Alert type="error" showIcon message={topError} style={{ marginTop: 12 }} /> : null}
        {!topError && preview.rows.length > 0 ? (
          <>
            <Typography.Title level={5} style={{ marginTop: 16 }}>
              <FileSearchOutlined /> {t('clientImport.preview', { count: preview.rows.length })}
            </Typography.Title>
            <Table
              size="small"
              rowKey="index"
              pagination={false}
              scroll={{ y: 260 }}
              dataSource={preview.rows}
              columns={[
                { title: '#', dataIndex: 'index', width: 60 },
                { title: t('pages.clients.email'), dataIndex: 'email' },
                { title: t('pages.clients.attachedInbounds'), dataIndex: 'inboundCount', width: 130 },
                {
                  title: t('status'),
                  width: 230,
                  render: (_, row) => {
                    if (row.errors.length > 0) {
                      return <Tag color="red">{errorText(row.errors)}</Tag>;
                    }
                    const reason = skippedByEmail.get(row.email.toLowerCase());
                    if (reason !== undefined) return <Tag color="orange">{reason || t('clientImport.skipped')}</Tag>;
                    if (result) return <Tag color="green">{t('clientImport.created')}</Tag>;
                    return <Tag color="blue">{t('clientImport.ready')}</Tag>;
                  },
                },
              ]}
            />
          </>
        ) : null}
      </Modal>
    </>
  );
}
