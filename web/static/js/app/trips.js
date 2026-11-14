/* Походы = маршруты. Планирование на время с оценкой каждой точки,
 * подсказками по лучшим часам, подбором мест по условиям («по погоде»)
 * и действиями вкладки «Походы». Подключается через actions.js. */
import { escapeHtml, icon, currentParams } from './util.js';
import { api } from './api.js';
import { confirmDialog, openDialog, toast } from './ui.js';
import { requireAuth } from './auth.js';
import { routeState, toggleRoute } from './route.js';

const VL_OFFSET_MS = 10 * 3600 * 1000;
const HORIZON_DAYS = 10;

const ACTIVITIES = [
  { id: 'walk', label: 'Прогулка' },
  { id: 'run', label: 'Пробежка' },
  { id: 'hike', label: 'Поход' },
  { id: 'sup', label: 'SUP' },
];

const STOP_MINUTES = [
  { value: 30, label: '30 минут' },
  { value: 60, label: '1 час' },
  { value: 90, label: '1,5 часа' },
  { value: 120, label: '2 часа' },
];

const VERDICT_LABEL = { go: 'можно выходить', caution: 'осторожно', no_go: 'не рекомендуется' };
const HOUR_STEP = 45; // запас на переезд между точками, если маршрут ещё не посчитан

const vlDayFmt = new Intl.DateTimeFormat('ru-RU', {
  timeZone: 'Asia/Vladivostok', weekday: 'short', day: 'numeric', month: 'short',
});
const vlWhenFmt = new Intl.DateTimeFormat('ru-RU', {
  timeZone: 'Asia/Vladivostok', weekday: 'short', day: 'numeric', month: 'long', hour: '2-digit', minute: '2-digit',
});

const vlDate = (date = new Date()) => new Date(date.getTime() + VL_OFFSET_MS).toISOString().slice(0, 10);
const vlHM = (iso) => {
  const shifted = new Date(Date.parse(iso) + VL_OFFSET_MS);
  return `${String(shifted.getUTCHours()).padStart(2, '0')}:${String(shifted.getUTCMinutes()).padStart(2, '0')}`;
};

function horizonDates() {
  const base = Date.parse(`${vlDate()}T00:00:00+10:00`);
  return Array.from({ length: HORIZON_DAYS }, (_, i) => {
    const day = new Date(base + i * 86400000);
    const label = vlDayFmt.format(day);
    const capped = label.charAt(0).toUpperCase() + label.slice(1);
    return { value: vlDate(day), label: i === 0 ? `Сегодня, ${capped}` : i === 1 ? `Завтра, ${capped}` : capped };
  });
}

const scoreClass = (verdict) => (verdict === 'go' || verdict === 'caution' ? `is-${verdict}` : verdict === 'no_go' ? 'is-no-go' : 'na');

const scoreChip = (score, verdict) =>
  `<span class="trip-score trip-score--${scoreClass(verdict)}" title="${verdict ? VERDICT_LABEL[verdict] : 'оценки пока нет'}">${score ?? '—'}</span>`;

const field = (label, role, options, value) => `
  <label class="field">
    <span class="field__label">${label}</span>
    <select class="input" data-role="${role}">
      ${options.map((o) => `<option value="${o.value}"${String(o.value) === String(value) ? ' selected' : ''}>${o.label}</option>`).join('')}
    </select>
  </label>`;

/* ── Планирование маршрута на время ─────────────────────────── */

export function openPlanDialog({ stops = null, activity = 'hike', date = '', time = '' } = {}) {
  const source = stops || routeState.stops.map((s) => ({ id: s.id, title: s.title }));
  if (!source.length) {
    toast('Сначала добавьте точки в маршрут', { type: 'danger' });
    return;
  }

  requireAuth('Войдите, чтобы планировать походы — планы хранятся в вашем профиле.').then((ok) => {
    if (ok) renderPlanDialog(source, { activity, date, time });
  });
}

function renderPlanDialog(source, { activity, date, time }) {
  const dates = horizonDates();
  const times = Array.from({ length: 17 }, (_, i) => {
    const h = String(i + 5).padStart(2, '0');
    return { value: h, label: `${h}:00` };
  });
  const multi = source.length > 1;

  const state = {
    activity: ACTIVITIES.some((a) => a.id === activity) ? activity : 'hike',
    date: dates.some((d) => d.value === date) ? date : dates[0].value,
    time: /^([5-9]|1\d|2[01])$/.test(time) ? time : '10',
    stopMinutes: 60,
  };

  const dlg = openDialog({
    title: 'Запланировать поход',
    lead: `Маршрут: ${source.map((s) => s.title).join(' → ')}. Оценим условия у каждой точки на время прибытия.`,
    className: 'trip-plan',
    wide: true,
    body: `
      <div class="trip-plan__controls">
        ${field('Занятие', 'activity', ACTIVITIES, state.activity)}
        ${field('День', 'date', dates, state.date)}
        ${field('Старт', 'time', times, state.time)}
        ${multi ? field('На точке', 'stop-minutes', STOP_MINUTES, state.stopMinutes) : ''}
      </div>
      <p class="trip-plan__note" data-role="note"></p>
      <ol class="trip-plan__stops" data-role="preview"></ol>
    `,
    footer: '<button type="button" class="btn btn--primary" data-role="save" disabled>Сохранить поход</button>',
    onMount(el, close, apiEl) {
      el.addEventListener('change', (event) => {
        const role = event.target.dataset.role;
        if (role === 'activity') state.activity = event.target.value;
        else if (role === 'date') state.date = event.target.value;
        else if (role === 'time') state.time = event.target.value;
        else if (role === 'stop-minutes') state.stopMinutes = Number(event.target.value);
        else return;
        rebuild();
      });

      let arrivals = [];
      let assessments = [];
      let request = 0;

      const legsMatchRoute = () =>
        multi &&
        routeState.stops.length === source.length &&
        routeState.stops.every((s, i) => s.id === source[i].id);

      const computeArrivals = () => {
        const start = Date.parse(`${state.date}T${state.time}:00:00+10:00`);
        const legs = legsMatchRoute() && routeState.result ? routeState.result.legs : [];
        let cursor = start;
        return source.map((_, i) => {
          const at = cursor;
          const leg = legs[i];
          cursor += ((leg ? leg.minutes : HOUR_STEP * (multi ? 1 : 0)) + (multi ? state.stopMinutes : 0)) * 60000;
          return at;
        });
      };

      const noteText = () => {
        if (multi && legsMatchRoute() && routeState.computing) return 'Считаем переезды между точками…';
        if (multi && legsMatchRoute() && !routeState.result) return `Переезды пока не посчитаны — время прибытия прикидочное, заложите переезды сами.`;
        if (multi && !legsMatchRoute()) return 'Время прибытия прикидочное: без учёта переездов.';
        return 'Оценка каждой точки — по почасовому прогнозу на время прибытия (горизонт 10 дней).';
      };

      const renderRows = () => {
        const preview = el.querySelector('[data-role="preview"]');
        preview.innerHTML = source
          .map(
            (stop, i) => `
          <li class="trip-plan__stop" data-index="${i}">
            <span class="trip-stop__num">${i + 1}</span>
            <div class="trip-plan__stop-main">
              <strong>${escapeHtml(stop.title)}</strong>
              <span class="trip-plan__stop-meta">прибытие ~${vlHM(new Date(arrivals[i]).toISOString())} · <span data-role="stop-status">считаем оценку…</span></span>
              <span class="trip-plan__stop-extra" data-role="stop-extra"></span>
            </div>
            <span data-role="stop-chip">${scoreChip(null, '')}</span>
          </li>`,
          )
          .join('');
      };

      const applyAssessments = () => {
        source.forEach((_, i) => {
          const row = el.querySelector(`[data-index="${i}"]`);
          if (!row) return;
          const status = row.querySelector('[data-role="stop-status"]');
          const extra = row.querySelector('[data-role="stop-extra"]');
          const chipBox = row.querySelector('[data-role="stop-chip"]');
          const result = assessments[i];

          if (!result || !result.ok || !result.list.length) {
            chipBox.innerHTML = scoreChip(null, '');
            status.textContent = 'оценки на это время пока нет';
            extra.innerHTML = '';
            return;
          }

          const nearest = result.list.reduce((best, item) =>
            Math.abs(Date.parse(item.forecast_time) - arrivals[i]) < Math.abs(Date.parse(best.forecast_time) - arrivals[i]) ? item : best,
          );
          const best = result.list.reduce((top, item) => (item.comfort_score > top.comfort_score ? item : top));
          chipBox.innerHTML = scoreChip(nearest.comfort_score, nearest.verdict);
          status.textContent = `${VERDICT_LABEL[nearest.verdict] ?? ''}, ${vlHM(nearest.forecast_time)} по прогнозу`;

          const warning = (nearest.warnings || [])[0];
          const hint = [];
          if (warning) hint.push(`<span class="trip-plan__warn">${icon('triangle-alert', 'icon--sm')} ${escapeHtml(warning.message)}</span>`);
          if (best.comfort_score >= nearest.comfort_score + 15 && vlHM(best.forecast_time) !== vlHM(nearest.forecast_time)) {
            hint.push(`<span class="trip-plan__hint">${icon('sun', 'icon--sm')} лучшее время ${vlHM(best.forecast_time)} — там ${best.comfort_score} вместо ${nearest.comfort_score}</span>`);
          }
          extra.innerHTML = hint.join('');
        });
      };

      const rebuild = async () => {
        const myRequest = ++request;
        arrivals = computeArrivals();
        el.querySelector('[data-role="note"]').textContent = noteText();
        renderRows();
        const save = el.querySelector('[data-role="save"]');
        save.disabled = true;

        const results = await Promise.all(
          source.map(async (stop) => {
            try {
              const list = await api(
                `/forecast/place/${encodeURIComponent(stop.id)}?date=${state.date}&activity=${state.activity}&days=1`,
              );
              return { ok: true, list };
            } catch (error) {
              return { ok: false, error };
            }
          }),
        );
        if (myRequest !== request) return; // устаревший запрос

        assessments = results;
        applyAssessments();
        save.disabled = false;
      };

      el.querySelector('[data-role="save"]').addEventListener('click', async () => {
        const save = el.querySelector('[data-role="save"]');
        save.disabled = true;
        try {
          await api('/trips', {
            method: 'POST',
            body: {
              planned_at: new Date(arrivals[0]).toISOString(),
              stop_minutes: multi ? state.stopMinutes : 60,
              activity: state.activity,
              notes: '',
              stops: source.map((stop, i) => ({
                place_id: stop.id,
                arrive_at: new Date(arrivals[i]).toISOString(),
              })),
            },
          });
          close();
          toast('Поход запланирован', {
            type: 'success',
            action: { label: 'Открыть походы', onClick: () => { window.location.href = '/profile?tab=trips'; } },
          });
        } catch (error) {
          toast(error.message || 'Не получилось сохранить поход', { type: 'danger' });
          save.disabled = false;
        }
      });

      rebuild();
    },
  });

  return dlg;
}

/* ── Подбор мест по условиям («по погоде») ──────────────────── */

export function openTripPicker() {
  const dates = horizonDates();
  const state = { activity: 'hike', date: dates[0].value };

  openDialog({
    title: 'Куда поехать по погоде',
    lead: 'Сравниваем почасовые оценки условий во всех местах: балл, лучшее время дня и число комфортных часов.',
    className: 'trip-picker',
    wide: true,
    body: `
      <div class="trip-picker__controls">
        ${field('Занятие', 'activity', ACTIVITIES, state.activity)}
        ${field('День', 'date', dates, state.date)}
      </div>
      <p class="trip-picker__hint">Балл — комфорт условий от 0 до 100. «Хороших часов» — сколько часов в день оценка 70 и выше.</p>
      <ul class="trip-picker__results" data-role="results"></ul>
    `,
    onMount(el) {
      let request = 0;

      const load = async () => {
        const myRequest = ++request;
        const box = el.querySelector('[data-role="results"]');
        box.innerHTML = `<li class="trip-picker__state">${icon('refresh-cw', 'icon--sm')} Считаем оценки…</li>`;
        try {
          const picks = await api(`/forecast/picks?activity=${state.activity}&date=${state.date}`);
          if (myRequest !== request) return;
          if (!picks.length) {
            box.innerHTML = `<li class="trip-picker__state">Оценок на этот день пока нет — прогноз ещё не посчитан. Попробуйте другой день.</li>`;
            return;
          }
          box.innerHTML = picks
            .map(
              (pick) => `
            <li class="trip-pick">
              <span data-role="pick-chip">${scoreChip(pick.score, pick.verdict)}</span>
              <div class="trip-pick__main">
                <a class="trip-pick__name" href="${pick.place_url}">${escapeHtml(pick.place_name)}</a>
                <span class="trip-pick__meta">
                  ${escapeHtml(pick.district || 'Приморье')} · лучшее время ${vlHM(pick.forecast_time)} · хороших часов ${pick.good_hours} из ${pick.total_hours}
                </span>
                ${pick.summary ? `<span class="trip-pick__summary">${escapeHtml(pick.summary)}</span>` : ''}
              </div>
              <div class="trip-pick__actions">
                <button type="button" class="btn btn--sm btn--outline" data-pick-plan="${pick.place_id}"
                  data-pick-name="${escapeHtml(pick.place_name)}" data-pick-time="${vlHM(pick.forecast_time)}">Запланировать</button>
                <button type="button" class="btn btn--sm btn--ghost" data-pick-route="${pick.place_id}">В маршрут</button>
              </div>
            </li>`,
            )
            .join('');
        } catch (error) {
          if (myRequest !== request) return;
          box.innerHTML = `<li class="trip-picker__state">Не получилось загрузить оценки: ${escapeHtml(error.message || 'ошибка')}</li>`;
        }
      };

      el.addEventListener('change', (event) => {
        if (event.target.dataset.role === 'activity') state.activity = event.target.value;
        else if (event.target.dataset.role === 'date') state.date = event.target.value;
        else return;
        load();
      });

      el.addEventListener('click', (event) => {
        const routeBtn = event.target.closest('[data-pick-route]');
        if (routeBtn) {
          toggleRoute(routeBtn.dataset.pickRoute);
          routeBtn.textContent = 'Добавлено';
          routeBtn.disabled = true;
          return;
        }
        const planBtn = event.target.closest('[data-pick-plan]');
        if (planBtn) {
          const dialog = planBtn.closest('dialog');
          openPlanDialog({
            stops: [{ id: planBtn.dataset.pickPlan, title: planBtn.dataset.pickName }],
            activity: state.activity,
            date: state.date,
            time: planBtn.dataset.pickTime.slice(0, 2),
          });
          dialog?.close();
        }
      });

      load();
    },
  });
}

/* ── Действия вкладки «Походы» ──────────────────────────────── */

export function tripSetStatus(id, status) {
  return api(`/trips/${encodeURIComponent(id)}`, { method: 'PATCH', body: { status } })
    .then(() => {
      toast(status === 'done' ? 'Отметили: поход состоялся' : 'Поход отменён');
      htmx.ajax('GET', '/fragments/profile-tab?tab=trips', { target: '#profile-tab' });
    })
    .catch((error) => toast(error.message || 'Не получилось обновить поход', { type: 'danger' }));
}

export function tripCancel(id) {
  confirmDialog({
    title: 'Отменить поход?',
    text: 'Маршрут останется в списке как отменённый — удалить его нельзя.',
    confirmLabel: 'Отменить поход',
    danger: true,
  }).then((ok) => {
    if (ok) tripSetStatus(id, 'cancelled');
  });
}

/* ── Подсветка узлов запланированного похода в редакторе ──── */

function decorateRouteEditor(trip) {
  const container = document.querySelector('[data-role="route-editor"]');
  if (!container || container.dataset.tripDecorated === trip.id) return;

  const byPlace = new Map(trip.stops.map((stop) => [stop.place_id, stop]));

  const tryDecorate = () => {
    const rows = container.querySelectorAll('.route-stop');
    if (!rows.length) return false;
    container.dataset.tripDecorated = trip.id;

    let weakest = null;
    rows.forEach((row) => {
      const link = row.querySelector('a.route-stop__title');
      const match = link?.getAttribute('href')?.match(/\/place\/([^/?#]+)/);
      if (!match) return;
      const stop = byPlace.get(match[1]);
      if (!stop) return;

      const main = row.querySelector('div');
      const chip = document.createElement('span');
      chip.className = `trip-score trip-score--${scoreClass(stop.verdict)} route-stop__score`;
      chip.title = stop.verdict ? VERDICT_LABEL[stop.verdict] : 'оценки пока нет';
      chip.textContent = stop.score ?? '—';
      main?.append(chip);

      const warning = (stop.warnings || [])[0];
      if (warning) {
        const warn = document.createElement('div');
        warn.className = 'route-stop__warn';
        warn.textContent = warning.message;
        main?.append(warn);
      }

      if (stop.verdict === 'no_go' || (!weakest && stop.verdict === 'caution')) weakest = stop;
    });

    const note = document.createElement('div');
    note.className = 'trip-editor-note';
    const when = vlWhenFmt.format(new Date(trip.planned_at));
    const weakText = weakest
      ? `Самый слабый узел — «${escapeHtml(weakest.place_name)}»: ${weakest.score ?? '—'} баллов, ${VERDICT_LABEL[weakest.verdict]}.`
      : 'Все точки в норме.';
    note.innerHTML = `${icon('info', '')}<span>Открыт запланированный поход (${when}). ${weakText}</span>`;
    container.prepend(note);

    return true;
  };

  // Alpine рендерит список x-for после htmx-свопа — ждём появления строк.
  const timer = setInterval(() => {
    if (tryDecorate() || !document.body.contains(container)) clearInterval(timer);
  }, 300);
  setTimeout(() => clearInterval(timer), 8000);
}

function maybeDecorateRouteEditor() {
  const params = currentParams();
  const tripId = params.get('trip');
  if (params.get('panel') !== 'route' || !tripId) return;

  api(`/trips/${encodeURIComponent(tripId)}`)
    .then((trip) => decorateRouteEditor(trip))
    .catch(() => {});
}

document.body.addEventListener('htmx:afterSwap', (event) => {
  if (event.target && event.target.id === 'map-panel-inner') maybeDecorateRouteEditor();
});

document.addEventListener('DOMContentLoaded', maybeDecorateRouteEditor);
