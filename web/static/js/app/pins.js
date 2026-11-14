/* Радар «Сейчас»: создание метки — два шага, черновик шага в sessionStorage
 * (localStorage-обёртка), плюс доступ к запущенному компоненту извне. */
import { api } from './api.js';
import { formatDuration, isMobileLayout, storage } from './util.js';
import { toast } from './ui.js';
import { requireAuth } from './auth.js';
import { refreshPins, selectPin } from './nav.js';

const NOW_STEP_KEY = 'vlru-places:now-create';

export const pickerNode = () => document.querySelector('#map-stage [data-picker]');

export const setSheetState = (state) => {
  const panel = document.getElementById('map-panel');
  if (!panel || !isMobileLayout()) return;
  panel.dataset.sheet = state;
  const handle = panel.querySelector('.map-panel__handle');
  handle?.setAttribute('aria-expanded', String(state !== 'peek'));
};

export function nowCreateComponent() {
  const el = document.querySelector('[x-data="nowCreate"]');
  if (!el || !window.Alpine) return null;
  try {
    return window.Alpine.$data(el);
  } catch {
    return null;
  }
}

export function registerNowCreate() {
  Alpine.data('nowCreate', () => ({
    step: 'list',
    draft: { placeLabel: '', kind: '', title: '', startsInMin: 0, durationMin: 120, need: 0, note: '' },
    errors: {},
    sending: false,
    init() {
      const saved = storage.get(NOW_STEP_KEY, null);
      if (saved === 'place' || saved === 'details') {
        this.step = saved;
        if (saved === 'place') this.showPicker(true);
      }
    },
    showPicker(visible) {
      const node = pickerNode();
      if (node) node.hidden = !visible;
    },
    start() {
      requireAuth('Чтобы поставить метку, войдите — это займёт минуту.').then((ok) => {
        if (!ok) return;
        this.draft = { placeLabel: '', kind: '', title: '', startsInMin: 0, durationMin: 120, need: 0, note: '' };
        this.errors = {};
        this.step = 'place';
        storage.set(NOW_STEP_KEY, 'place');
        this.showPicker(true);
        setSheetState('full');
      });
    },
    cancel() {
      this.step = 'list';
      this.errors = {};
      storage.remove(NOW_STEP_KEY);
      this.showPicker(false);
    },
    submitPlace() {
      this.draft.coords = window.placesMap ? window.placesMap.getCenter() : null;
      this.step = 'details';
      storage.set(NOW_STEP_KEY, 'details');
      this.showPicker(false);
    },
    async submitDetails() {
      const errors = {};
      if (!this.draft.kind) errors.kind = 'Выберите тип метки';
      if (this.draft.title.trim().length < 5) errors.title = 'Опишите встречу хотя бы в пяти символах';
      this.errors = errors;
      if (Object.keys(errors).length) return;

      this.sending = true;
      try {
        const now = Date.now();
        const startsAt = new Date(now + Number(this.draft.startsInMin) * 60000);
        const endsAt = new Date(now + (Number(this.draft.startsInMin) + Number(this.draft.durationMin)) * 60000);
        const [lon, lat] = this.draft.coords || [0, 0];
        const pin = await api('/pins', {
          method: 'POST',
          body: {
            kind: this.draft.kind,
            title: this.draft.title.trim(),
            note: this.draft.note.trim(),
            place_label: this.draft.placeLabel.trim() || 'Точка на карте',
            lat,
            lon,
            starts_at: startsAt.toISOString(),
            ends_at: endsAt.toISOString(),
            need: Number(this.draft.need) || 0,
          },
        });
        storage.remove(NOW_STEP_KEY);
        this.step = 'list';
        toast(`Метка опубликована. Исчезнет через ${formatDuration((new Date(pin.ends_at) - Date.now()) / 60000)}`, { type: 'success' });
        await refreshPins();
        selectPin(pin.id);
      } catch (error) {
        console.error('Не удалось опубликовать метку', error);
        toast(error.status === 401 ? 'Войдите, чтобы опубликовать метку' : 'Не получилось опубликовать. Попробуйте ещё раз', { type: 'danger' });
      } finally {
        this.sending = false;
      }
    },
    destroy() {
      this.showPicker(false);
    },
  }));
}
