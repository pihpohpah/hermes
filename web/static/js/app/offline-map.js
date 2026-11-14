/* Профиль: офлайн-карта — скачать архивы целиком, обновить, удалить, очистить кеш. */
import { formatBytes } from './util.js';
import { confirmDialog, toast } from './ui.js';
import { clearChunkCache, copiesOutdated, deleteCopies, downloadMap, offlineStatus, serverArchives } from './map-tiles.js';

const savedDate = (ts) => new Date(ts).toLocaleDateString('ru-RU', { day: 'numeric', month: 'long' });

export function registerOfflineMap() {
  // AbortController — вне реактивного состояния: через прокси Alpine его методы падают.
  let controller = null;

  Alpine.data('offlineMap', () => ({
    loading: true,
    downloading: false,
    received: 0,
    total: 0,
    copyBytes: 0,
    savedAt: null,
    chunkBytes: 0,
    serverBytes: 0,
    outdated: false,
    unavailable: false,

    async init() {
      await this.refresh();
      this.loading = false;
    },

    async refresh() {
      try {
        Object.assign(this, await offlineStatus());
      } catch (error) {
        console.warn('Офлайн-карта: не удалось прочитать хранилище', error);
      }
      try {
        const heads = await serverArchives();
        this.serverBytes = heads.reduce((sum, head) => sum + head.bytes, 0);
        this.outdated = this.copyBytes > 0 && (await copiesOutdated());
        this.unavailable = false;
      } catch (error) {
        console.warn('Офлайн-карта: сервер карты недоступен', error);
        this.unavailable = true;
      }
    },

    get hasCopy() {
      return this.copyBytes > 0;
    },

    get percent() {
      return this.total ? Math.min(100, Math.floor((this.received / this.total) * 100)) : 0;
    },

    get progressLabel() {
      return this.total ? `${formatBytes(this.received)} из ${formatBytes(this.total)}` : formatBytes(this.received);
    },

    get summary() {
      if (this.hasCopy) {
        const base = `Карта на устройстве: ${formatBytes(this.copyBytes)}, сохранена ${savedDate(this.savedAt)}. Без интернета она откроется из этой копии.`;
        return this.outdated ? `${base} На сервере есть обновление.` : base;
      }
      if (this.unavailable) return 'Сервер карты сейчас недоступен — скачать карту не получится.';
      const size = this.serverBytes ? ` (${formatBytes(this.serverBytes)})` : '';
      return `Карта открывается через интернет. Скачайте её целиком${size}, чтобы она работала в походе, на островах и в море без связи.`;
    },

    get cacheNote() {
      return this.chunkBytes ? `Просмотренные области сохраняются сами: ${formatBytes(this.chunkBytes)}.` : '';
    },

    async download() {
      controller = new AbortController();
      this.downloading = true;
      this.received = 0;
      this.total = this.serverBytes;
      try {
        await downloadMap({
          signal: controller.signal,
          onProgress: (received, total) => {
            this.received = received;
            this.total = total;
          },
        });
        toast('Карта сохранена на устройстве', { type: 'success' });
      } catch (error) {
        if (error?.name === 'AbortError') {
          toast('Скачивание отменено');
        } else if (error?.name === 'QuotaExceededError') {
          console.warn('Офлайн-карта: не хватило места', error);
          toast('Не хватает места на устройстве', { type: 'danger' });
        } else {
          console.warn('Офлайн-карта: скачивание не удалось', error);
          toast('Не удалось скачать карту — попробуйте позже', { type: 'danger' });
        }
      } finally {
        this.downloading = false;
        controller = null;
        await this.refresh();
      }
    },

    cancel() {
      controller?.abort();
    },

    async remove() {
      const confirmed = await confirmDialog({
        title: 'Удалить офлайн-карту?',
        text: 'Без интернета карта откроется только в уже просмотренных местах.',
        confirmLabel: 'Удалить',
        danger: true,
      });
      if (!confirmed) return;
      try {
        await deleteCopies();
        toast('Офлайн-карта удалена');
      } catch (error) {
        console.warn('Офлайн-карта: не удалось удалить', error);
        toast('Не удалось удалить карту', { type: 'danger' });
      }
      await this.refresh();
    },

    async clearCache() {
      try {
        await clearChunkCache();
        toast('Кеш просмотренных областей очищен');
      } catch (error) {
        console.warn('Офлайн-карта: не удалось очистить кеш', error);
        toast('Не удалось очистить кеш', { type: 'danger' });
      }
      await this.refresh();
    },
  }));
}
