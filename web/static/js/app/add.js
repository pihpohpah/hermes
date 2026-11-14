/* Мастер добавления места/события/правки: три шага, черновик в localStorage,
 * фото уходят на сервер сразу при выборе. */
import { api } from './api.js';
import { storage } from './util.js';
import { toast } from './ui.js';
import { isLoggedIn, requireAuth } from './auth.js';
import { CATEGORY_LABELS, SCENARIO_LABELS, TYPE_LABELS } from './taxonomy.js';

const ADD_DRAFT_KEY = 'vlru-places:add-draft';
const DESCRIPTION_MIN = 30;
const MAX_PHOTOS = 5;
let photoSeq = 0;
const photoKey = () => `p${Date.now().toString(36)}-${++photoSeq}`;

export function registerAddMaster() {
  Alpine.data('addMaster', () => ({
    step: 1,
    steps: ['Что', 'Подробности', 'Отправка'],
    draft: null,
    errors: {},
    sending: false,
    pickerVisible: false,
    pickerMap: null,
    pickerMarker: null,
    init() {
      const params = new URLSearchParams(location.search);
      this.step = Math.min(3, Math.max(1, Number(params.get('step')) || 1));

      const saved = storage.get(ADD_DRAFT_KEY, null);
      const fixItem = this.$el.dataset.fixItem || '';
      this.draft = {
        type: params.get('type') || 'place',
        scenario: params.get('scenario') || 'city',
        category: '',
        fixReasons: [],
        title: fixItem,
        address: '',
        coords: null,
        date: '',
        time: '',
        description: '',
        hours: '',
        price: '',
        link: '',
        consent: false,
        photos: [],
        ...(saved && saved.v === 1 ? saved.draft : {}),
        // Обрыв загрузки не переживает: файлов уже нет, серверные URL — есть.
        photos: (saved?.draft?.photos || []).filter((p) => p && p.src).map((p) => ({ ...p, key: photoKey() })),
      };
      this.$watch('draft', () => this.persist(), { deep: true });
    },
    persist() {
      storage.set(ADD_DRAFT_KEY, {
        v: 1,
        draft: {
          ...this.draft,
          photos: this.draft.photos
            .filter((p) => p.src)
            .map(({ src, src_sm, width, height, url }) => ({ src, src_sm, width, height, url })),
        },
      });
    },
    get photosBusy() {
      return this.draft.photos.some((p) => p.uploading);
    },
    gotoStep(n) {
      this.step = Math.min(3, Math.max(1, n));
      const params = new URLSearchParams(location.search);
      if (this.step === 1) params.delete('step');
      else params.set('step', String(this.step));
      const query = params.toString();
      history.pushState({}, '', `${location.pathname}${query ? `?${query}` : ''}`);
      this.$nextTick(() => {
        this.$refs.stepTitle?.focus();
        if (this.step === 2) this.scrollTop();
      });
    },
    scrollTop() {
      window.scrollTo({ top: 0, behavior: 'smooth' });
    },
    typeLabel() {
      return TYPE_LABELS[this.draft.type] || this.draft.type;
    },
    scenarioLabel() {
      return SCENARIO_LABELS[this.draft.scenario] || this.draft.scenario;
    },
    categoryLabel() {
      const byType = this.draft.type === 'event' ? CATEGORY_LABELS.event : CATEGORY_LABELS[this.draft.scenario];
      return byType?.[this.draft.category] || this.draft.category || '—';
    },
    whenLabel() {
      if (!this.draft.date) return '';
      const [year, month, day] = this.draft.date.split('-').map(Number);
      const date = new Date(year, month - 1, day);
      const months = ['января', 'февраля', 'марта', 'апреля', 'мая', 'июня', 'июля', 'августа', 'сентября', 'октября', 'ноября', 'декабря'];
      return `${day} ${months[month - 1]}${this.draft.time ? ` в ${this.draft.time}` : ''}`;
    },
    validate(step) {
      const errors = {};
      if (step === 1) {
        if (this.draft.type === 'fix') {
          if (!this.draft.fixReasons.length) errors.fixReasons = 'Отметьте хотя бы один пункт';
        } else if (!this.draft.category) {
          errors.category = 'Выберите категорию';
        }
      }
      if (step === 2) {
        if (this.draft.title.trim().length < 3) errors.title = 'Напишите название — хотя бы три буквы';
        if (this.draft.type !== 'fix' && this.draft.address.trim().length < 3 && !this.draft.coords) {
          errors.address = 'Укажите адрес, ориентир или точку на карте';
        }
        if (this.draft.type === 'event') {
          if (!this.draft.date) errors.date = 'Выберите дату';
          if (!this.draft.time) errors.time = 'Укажите время начала';
        }
        const min = this.draft.type === 'fix' ? 10 : DESCRIPTION_MIN;
        if (this.draft.description.trim().length < min) {
          errors.description = `Добавьте пару предложений — хотя бы ${min} символов`;
        }
      }
      if (step === 3 && !this.draft.consent) {
        errors.consent = 'Подтвердите, что информация достоверна';
      }
      this.errors = errors;
      return !Object.keys(errors).length;
    },
    submit() {
      if (this.sending) return;
      if (!this.validate(this.step)) return;
      if (this.step < 3) {
        this.gotoStep(this.step + 1);
        return;
      }
      this.send();
    },
    // Фото уходят на сервер сразу при выборе: в черновике лежит либо
    // заглушка с object URL (uploading), либо серверный {src, src_sm}.
    // key у каждого превью своё — иначе x-for по индексам путается при
    // удалении оборвавшихся загрузок.
    addPhotos(event) {
      const files = Array.from(event.target.files || []).filter((file) => file.type.startsWith('image/'));
      const room = MAX_PHOTOS - this.draft.photos.length;
      if (files.length > room) toast(`Можно добавить ещё ${room}. Лишние фото не загрузили`, { type: 'danger' });
      files.slice(0, room).forEach((file) => this.uploadPhoto(file));
      event.target.value = '';
    },
    async uploadPhoto(file) {
      const localURL = URL.createObjectURL(file);
      this.draft.photos.push({ key: photoKey(), file, url: localURL, uploading: true });
      const index = this.draft.photos.length - 1;
      try {
        const data = await api('/photos', { method: 'POST', formData: (() => { const fd = new FormData(); fd.append('file', file); return fd; })() });
        // Меняем запись через индекс массива: локальный объект после push —
        // сырая ссылка, мутировать его реактивность не заметит.
        this.draft.photos[index] = {
          ...this.draft.photos[index],
          file: null,
          uploading: false,
          src: data.url,
          src_sm: data.url_sm,
          width: data.width,
          height: data.height,
          url: data.url_sm || data.url,
        };
        URL.revokeObjectURL(localURL);
      } catch (error) {
        console.warn('Фото не загрузилось', error);
        if (this.draft.photos[index] && this.draft.photos[index].file === file) this.draft.photos.splice(index, 1);
        URL.revokeObjectURL(localURL);
        toast('Фото не загрузилось — попробуйте другое или меньший размер', { type: 'danger' });
      }
    },
    removePhoto(index) {
      const [removed] = this.draft.photos.splice(index, 1);
      if (removed?.file) URL.revokeObjectURL(removed.url);
    },
    togglePicker() {
      this.pickerVisible = !this.pickerVisible;
      if (!this.pickerVisible) {
        this.draft.coords = null;
        this.destroyPicker();
        return;
      }
      this.$nextTick(() => {
        const canvas = this.$el.querySelector('[data-picker-map]');
        if (!canvas || !window.maplibregl) {
          this.pickerVisible = false;
          toast('Карта недоступна — укажите адрес текстом', { type: 'danger' });
          return;
        }
        try {
          const center = this.draft.coords || [131.8855, 43.1155];
          this.pickerMap = new window.maplibregl.Map({
            container: canvas,
            style: '/map/style.json',
            center,
            zoom: 12,
            dragRotate: false,
            pitchWithRotate: false,
            attributionControl: { compact: true },
          });
          this.pickerMap.on('click', (e) => this.setPickerCoords(e.lngLat.toArray()));
          this.pickerMap.on('load', () => {
            if (this.draft.coords) this.setPickerCoords(this.draft.coords);
          });
        } catch (error) {
          console.warn('Карта выбора точки недоступна', error);
          this.pickerVisible = false;
          toast('Карта недоступна — укажите адрес текстом', { type: 'danger' });
        }
      });
    },
    setPickerCoords(coords) {
      this.draft.coords = coords;
      if (!this.pickerMarker && this.pickerMap) {
        this.pickerMarker = new window.maplibregl.Marker().setLngLat(coords).addTo(this.pickerMap);
      } else {
        this.pickerMarker?.setLngLat(coords);
      }
    },
    destroyPicker() {
      this.pickerMap?.remove();
      this.pickerMap = null;
      this.pickerMarker = null;
    },
    async send() {
      if (this.draft.photos.some((p) => p.uploading)) {
        toast('Подождите, фото ещё загружаются', { type: 'danger' });
        return;
      }
      const ok = await requireAuth('Войдите, чтобы следить за статусом предложения в профиле.');
      if (!ok) return;
      if (!isLoggedIn()) return;

      this.sending = true;
      try {
        const submission = {
          type: this.draft.type,
          scenario: this.draft.type === 'event' ? '' : this.draft.scenario,
          category: this.draft.category,
          title: this.draft.title.trim(),
          description: this.draft.description.trim(),
          address: this.draft.address.trim(),
          coords: this.draft.coords ? [this.draft.coords[0], this.draft.coords[1]] : null,
          date: this.draft.date,
          time: this.draft.time,
          hours: this.draft.hours,
          price: this.draft.price,
          link: this.draft.link,
          fix_reasons: this.draft.fixReasons,
          photos: this.draft.photos.map(({ src, src_sm, width, height }) => ({ src, src_sm, width, height })),
        };
        await api('/submissions', { method: 'POST', body: submission });
        this.draft.photos.forEach((photo) => URL.revokeObjectURL(photo.url));
        storage.remove(ADD_DRAFT_KEY);
        location.href = `/add/success?type=${encodeURIComponent(this.draft.type)}`;
      } catch (error) {
        console.error('Не удалось отправить предложение', error);
        toast('Не получилось отправить. Проверьте соединение и попробуйте ещё раз', { type: 'danger' });
      } finally {
        this.sending = false;
      }
    },
    destroy() {
      this.destroyPicker();
    },
  }));
}
