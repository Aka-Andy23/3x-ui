import { describe, it, expect, vi } from 'vitest';
import { fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import ClientFormModal from '@/pages/clients/ClientFormModal';
import type { InboundOption } from '@/hooks/useClients';
import { renderWithProviders } from './test-utils';

// ClientFormModal reads server state via react-query (useFail2banStatusQuery),
// so it needs a QueryClientProvider on top of the shared ThemeProvider wrapper.
function renderModal(inbounds: InboundOption[] = []) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return renderWithProviders(
    <QueryClientProvider client={queryClient}>
      <ClientFormModal
        open
        mode="add"
        client={null}
        inbounds={inbounds}
        save={vi.fn().mockResolvedValue(null)}
        onOpenChange={() => {}}
      />
    </QueryClientProvider>,
  );
}

function openCredentialsTab() {
  const next = Array.from(document.querySelectorAll('button'))
    .find((button) => (button.textContent ?? '').trim() === 'Next');
  if (!next) throw new Error('Next button not found');
  fireEvent.click(next);
}

function tooltipIconForLabel(label: string): HTMLElement {
  const labelEl = Array.from(document.querySelectorAll('.ant-form-item-label label'))
    .find((l) => (l.textContent ?? '').trim() === label);
  const item = labelEl?.closest('.ant-form-item') as HTMLElement | null;
  if (!item) throw new Error(`Form item not found for label: ${label}`);
  const tip = item.querySelector('.ant-form-item-tooltip') as HTMLElement | null;
  if (!tip) throw new Error(`No tooltip on form item: ${label}`);
  return tip;
}

describe('ClientFormModal credential tooltips', () => {
  it('explains that the Password field is only consumed by Trojan/Shadowsocks', async () => {
    renderModal();
    openCredentialsTab();

    const tip = tooltipIconForLabel('Password');
    fireEvent.mouseEnter(tip);

    await waitFor(() => {
      expect(document.body.textContent).toContain(
        'Only used by Trojan and Shadowsocks clients; ignored for VLESS, VMess, Hysteria, and WireGuard.',
      );
    });
  });

  it('explains that Hysteria Auth is the credential Hysteria actually uses', async () => {
    renderModal();
    openCredentialsTab();

    const tip = tooltipIconForLabel('Hysteria Auth');
    fireEvent.mouseEnter(tip);

    await waitFor(() => {
      expect(document.body.textContent).toContain(
        'Credential used only by Hysteria clients. Trojan and Shadowsocks use the Password field instead.',
      );
    });
  });

  it('uses a four-step wizard and blocks subscription setup until an inbound is selected', () => {
    renderModal();
    openCredentialsTab();
    const next = Array.from(document.querySelectorAll('button'))
      .find((button) => (button.textContent ?? '').trim() === 'Next');
    if (!next) throw new Error('Next button not found');
    fireEvent.click(next);
    const active = document.querySelector('.ant-steps-item-process');
    expect(active?.textContent).toContain('Nodes and inbounds');
  });

  it('opens manual JSON setup after selecting an inbound', async () => {
    renderModal([{
      id: 7,
      protocol: 'vless',
      tag: 'in-vless',
      remark: 'Reality',
      enable: true,
    }]);
    openCredentialsTab();
    const selectAll = Array.from(document.querySelectorAll('button'))
      .find((button) => (button.textContent ?? '').trim() === 'Select all');
    if (!selectAll) throw new Error('Select all button not found');
    fireEvent.click(selectAll);
    const next = Array.from(document.querySelectorAll('button'))
      .find((button) => (button.textContent ?? '').trim() === 'Next');
    if (!next) throw new Error('Next button not found');
    fireEvent.click(next);
    const addManual = await waitFor(() => Array.from(document.querySelectorAll('button'))
      .find((button) => (button.textContent ?? '').trim() === 'Add manual JSON'));
    if (!addManual) throw new Error('Add manual JSON button not found');
    fireEvent.click(addManual);
    await waitFor(() => {
      expect(document.body.textContent).toContain('Manual JSON');
      expect(document.querySelector('.json-editor-host')).not.toBeNull();
    });
  });
});
