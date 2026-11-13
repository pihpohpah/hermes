(function () {
  const { icon } = window.VL_ICONS;

  const mapDrawing = `
    <svg class="map-drawing" viewBox="0 0 1600 1000" aria-hidden="true">
      <rect width="1600" height="1000" fill="var(--color-map-water)"/>
      <path class="water-line" d="M-50 720C250 650 380 790 600 730S920 670 1080 760s360 30 570-30"/>
      <path class="water-line" d="M-40 790C210 730 400 860 620 800s360-50 520 30 360 40 530-20"/>
      <path class="water-line" d="M950 400c180-70 300-10 520-80M1030 460c170-60 330-10 520-70"/>
      <path class="land" d="M0 0h1600v225c-140 25-245 30-355 95-105 62-177 64-258 120-69 48-90 121-130 185-34 55-58 119-115 148-68 35-127 6-155-53-28-60-20-121-82-154-67-36-122-5-182 28C227 665 115 659 0 678Z"/>
      <path class="land" d="M1045 695c78-57 169-59 242-22 58 30 94 9 151 39 75 40 119 128 102 205-13 59-54 75-107 83H979c-49-40-78-92-66-148 11-52 53-106 132-157Z"/>
      <path class="land" d="M1315 480c47-28 112-22 143 14 27 32 5 72-42 81-52 10-117-10-128-44-6-19 4-36 27-51Z"/>
      <path class="park" d="M420 150c85-51 189-22 211 47 19 61-34 120-105 120-97 0-176-108-106-167Z"/>
      <path class="park" d="M815 192c56-33 140-8 151 48 10 49-43 100-103 86-76-17-96-102-48-134Z"/>
      <path class="park" d="M1080 732c64-40 147-27 180 24 32 50-3 121-74 130-82 11-174-102-106-154Z"/>
      <path class="park" d="M545 494c35-28 89-8 91 33 2 36-35 68-70 52-41-18-54-57-21-85Z"/>
      <path class="road" d="M740 50c-20 128-6 231 5 326 11 92-19 183-4 290"/>
      <path class="road" d="M1020 140c-87 92-157 130-223 197-53 54-70 117-111 165"/>
      <path class="road" d="M185 440c175 25 279-3 402 39 83 29 135 19 205-11"/>
      <path class="road" d="M1110 760c80 20 161 69 242 149"/>
      <path class="road road--bridge" d="M780 626c130 32 225 71 331 140"/>
      <path class="road road--bridge" d="M990 393c90 52 130 120 169 197"/>
      <path class="road road--small" d="M656 241c61 31 107 23 167-5M600 347c79 23 175 12 244-28M578 535c63-22 128-19 192 9M661 606c49-13 92-5 130 17M804 446c53 21 102 18 151-3M1118 234c-37 43-60 94-77 147"/>
      <path class="road road--small" d="M291 283c88 22 145 9 212-24M302 355c72 13 136 7 197-16M1150 815c42-21 95-23 145-5"/>
      <text class="map-label map-label--water" x="180" y="735">Амурский залив</text>
      <text class="map-label map-label--water" x="1050" y="545">Уссурийский залив</text>
      <text class="map-label map-label--water" x="440" y="915" font-size="18">пролив Босфор Восточный</text>
      <text class="map-label" x="650" y="390">Центр</text>
      <text class="map-label" x="410" y="245">Снеговая Падь</text>
      <text class="map-label" x="830" y="240">Садгород</text>
      <text class="map-label" x="1060" y="185">Шамора</text>
      <text class="map-label" x="350" y="525">Первая Речка</text>
      <text class="map-label" x="770" y="575">Золотой Рог</text>
      <text class="map-label" x="1070" y="845">о. Русский</text>
      <text class="map-label map-label--small" x="1245" y="520">о. Попова</text>
      <text class="map-label map-label--small" x="610" y="690">Токаревская кошка</text>
      <text class="map-label map-label--small" x="1020" y="740">Вятлина</text>
      <text class="map-label map-label--small" x="535" y="115">Патрокл</text>
    </svg>`;

  class VLMap {
    constructor(viewport, options) {
      this.viewport = viewport;
      this.options = options || {};
      this.items = [];
      this.selectedId = null;
      this.zoom = 1;
      this.x = 0;
      this.y = 0;
      this.scale = 1;
      this.baseScale = 1;
      this.pointers = new Map();
      this.dragged = false;
      this.destroyed = false;

      this.world = document.createElement('div');
      this.world.className = 'map-world';
      this.world.innerHTML = mapDrawing;
      this.viewport.appendChild(this.world);
      this.markerLayer = document.createElement('div');
      this.world.appendChild(this.markerLayer);

      this.onPointerDown = this.onPointerDown.bind(this);
      this.onPointerMove = this.onPointerMove.bind(this);
      this.onPointerUp = this.onPointerUp.bind(this);
      this.onWheel = this.onWheel.bind(this);
      this.onKeyDown = this.onKeyDown.bind(this);
      this.onResize = this.onResize.bind(this);

      this.viewport.addEventListener('pointerdown', this.onPointerDown);
      this.viewport.addEventListener('pointermove', this.onPointerMove);
      this.viewport.addEventListener('pointerup', this.onPointerUp);
      this.viewport.addEventListener('pointercancel', this.onPointerUp);
      this.viewport.addEventListener('wheel', this.onWheel, { passive: false });
      this.viewport.addEventListener('keydown', this.onKeyDown);
      this.markerLayer.addEventListener('click', (event) => this.onMarkerClick(event));
      this.resizeObserver = new ResizeObserver(this.onResize);
      this.resizeObserver.observe(this.viewport);
      requestAnimationFrame(() => !this.destroyed && this.fit());
    }

    onResize() {
      const previousBase = this.baseScale;
      this.baseScale = this.getBaseScale();
      if (!this.hasFit || Math.abs(previousBase - this.baseScale) > 0.01) {
        this.fit();
      }
    }

    getBaseScale() {
      const rect = this.viewport.getBoundingClientRect();
      return Math.max(rect.width / 1600, rect.height / 1000) || 1;
    }

    fit() {
      this.baseScale = this.getBaseScale();
      this.zoom = 1;
      this.scale = this.baseScale;
      const rect = this.viewport.getBoundingClientRect();
      this.x = (rect.width - 1600 * this.scale) / 2;
      this.y = (rect.height - 1000 * this.scale) / 2;
      this.hasFit = true;
      this.applyTransform();
      this.renderMarkers();
      this.emitZoom();
    }

    setItems(items, selectedId) {
      this.items = items || [];
      this.selectedId = selectedId || null;
      this.renderMarkers();
    }

    setSelected(id) {
      this.selectedId = id || null;
      this.markerLayer.querySelectorAll('[data-place-id]').forEach((marker) => {
        marker.classList.toggle('is-selected', marker.dataset.placeId === this.selectedId);
        marker.setAttribute('aria-pressed', String(marker.dataset.placeId === this.selectedId));
      });
    }

    renderMarkers() {
      if (!this.markerLayer) return;
      const grouped = this.zoom < 1.35;
      let markers = [];

      if (grouped) {
        const groups = new Map();
        this.items.forEach((item) => {
          const key = `${item.scenario}-${item.district}`;
          if (!groups.has(key)) groups.set(key, []);
          groups.get(key).push(item);
        });
        groups.forEach((items) => {
          if (items.length === 1) {
            markers.push(this.placeMarker(items[0]));
          } else {
            const x = items.reduce((sum, item) => sum + item.coords.x, 0) / items.length;
            const y = items.reduce((sum, item) => sum + item.coords.y, 0) / items.length;
            markers.push(this.clusterMarker(items, x, y));
          }
        });
      } else {
        markers = this.items.map((item) => this.placeMarker(item));
      }

      this.markerLayer.innerHTML = markers.join('');
      this.applyTransform();
    }

    placeMarker(item) {
      const markerIcon = item.kind === 'event' ? 'calendar' : ({ city: 'building', nature: 'tree', sea: 'wave' }[item.scenario]);
      const selected = item.id === this.selectedId;
      return `<button class="map-marker map-marker--${item.kind === 'event' ? 'event' : item.scenario}${selected ? ' is-selected' : ''}" style="left:${item.coords.x}px;top:${item.coords.y}px" data-place-id="${item.id}" type="button" aria-label="${this.escape(item.title)}" aria-pressed="${selected}" title="${this.escape(item.title)}"><span class="map-marker__shape">${icon(markerIcon)}</span></button>`;
    }

    clusterMarker(items, x, y) {
      const label = `${items.length} ${items.length < 5 ? 'точки' : 'точек'}: ${this.escape(items[0].district)}`;
      return `<button class="map-marker map-marker--cluster" style="left:${x}px;top:${y}px" type="button" data-cluster-x="${x}" data-cluster-y="${y}" aria-label="${label}" title="${label}"><span class="map-marker__shape"><span>${items.length}<small>точек</small></span></span></button>`;
    }

    onMarkerClick(event) {
      const place = event.target.closest('[data-place-id]');
      if (place) {
        event.stopPropagation();
        if (this.options.onSelect) this.options.onSelect(place.dataset.placeId);
        return;
      }
      const cluster = event.target.closest('[data-cluster-x]');
      if (cluster) {
        event.stopPropagation();
        this.zoomAt(Math.max(1.55, this.zoom * 1.7), Number(cluster.dataset.clusterX), Number(cluster.dataset.clusterY), true);
      }
    }

    onPointerDown(event) {
      if (event.button !== undefined && event.button !== 0) return;
      this.viewport.setPointerCapture(event.pointerId);
      this.pointers.set(event.pointerId, { x: event.clientX, y: event.clientY });
      this.dragStart = { x: event.clientX, y: event.clientY, mapX: this.x, mapY: this.y };
      this.dragged = false;
      this.viewport.classList.add('is-dragging');
      if (this.pointers.size === 2) {
        const points = [...this.pointers.values()];
        this.pinchStart = {
          distance: Math.hypot(points[0].x - points[1].x, points[0].y - points[1].y),
          zoom: this.zoom,
        };
      }
    }

    onPointerMove(event) {
      if (!this.pointers.has(event.pointerId)) return;
      this.pointers.set(event.pointerId, { x: event.clientX, y: event.clientY });
      if (this.pointers.size === 2 && this.pinchStart) {
        const points = [...this.pointers.values()];
        const distance = Math.hypot(points[0].x - points[1].x, points[0].y - points[1].y);
        if (distance > 0 && this.pinchStart.distance > 0) {
          const rect = this.viewport.getBoundingClientRect();
          const clientX = (points[0].x + points[1].x) / 2 - rect.left;
          const clientY = (points[0].y + points[1].y) / 2 - rect.top;
          this.setZoom(this.pinchStart.zoom * distance / this.pinchStart.distance, clientX, clientY);
          this.dragged = true;
        }
        return;
      }
      if (!this.dragStart) return;
      const dx = event.clientX - this.dragStart.x;
      const dy = event.clientY - this.dragStart.y;
      if (Math.abs(dx) + Math.abs(dy) > 5) this.dragged = true;
      this.x = this.dragStart.mapX + dx;
      this.y = this.dragStart.mapY + dy;
      this.clampPan();
      this.applyTransform();
    }

    onPointerUp(event) {
      this.pointers.delete(event.pointerId);
      if (this.viewport.hasPointerCapture(event.pointerId)) this.viewport.releasePointerCapture(event.pointerId);
      if (!this.pointers.size) {
        this.dragStart = null;
        this.pinchStart = null;
        this.viewport.classList.remove('is-dragging');
      }
    }

    onWheel(event) {
      event.preventDefault();
      const rect = this.viewport.getBoundingClientRect();
      const factor = event.deltaY > 0 ? 0.9 : 1.1;
      this.setZoom(this.zoom * factor, event.clientX - rect.left, event.clientY - rect.top);
    }

    onKeyDown(event) {
      const step = 60;
      if (event.key === '+' || event.key === '=') {
        event.preventDefault();
        this.zoomIn();
      } else if (event.key === '-') {
        event.preventDefault();
        this.zoomOut();
      } else if (event.key.startsWith('Arrow')) {
        event.preventDefault();
        if (event.key === 'ArrowLeft') this.x += step;
        if (event.key === 'ArrowRight') this.x -= step;
        if (event.key === 'ArrowUp') this.y += step;
        if (event.key === 'ArrowDown') this.y -= step;
        this.clampPan();
        this.applyTransform();
      } else if (event.key === '0') {
        event.preventDefault();
        this.fit();
      }
    }

    zoomIn() {
      const rect = this.viewport.getBoundingClientRect();
      this.setZoom(this.zoom * 1.3, rect.width / 2, rect.height / 2);
    }

    zoomOut() {
      const rect = this.viewport.getBoundingClientRect();
      this.setZoom(this.zoom / 1.3, rect.width / 2, rect.height / 2);
    }

    setZoom(nextZoom, screenX, screenY) {
      const oldScale = this.scale;
      this.zoom = Math.min(3.2, Math.max(0.75, nextZoom));
      const newScale = this.baseScale * this.zoom;
      const worldX = (screenX - this.x) / oldScale;
      const worldY = (screenY - this.y) / oldScale;
      this.scale = newScale;
      this.x = screenX - worldX * newScale;
      this.y = screenY - worldY * newScale;
      this.clampPan();
      this.applyTransform();
      this.renderMarkers();
      this.emitZoom();
    }

    zoomAt(nextZoom, worldX, worldY, center) {
      const rect = this.viewport.getBoundingClientRect();
      this.zoom = Math.min(3.2, Math.max(0.75, nextZoom));
      this.scale = this.baseScale * this.zoom;
      if (center) {
        this.x = rect.width / 2 - worldX * this.scale;
        this.y = rect.height / 2 - worldY * this.scale;
      }
      this.clampPan();
      this.applyTransform();
      this.renderMarkers();
      this.emitZoom();
    }

    centerOn(item, nextZoom) {
      if (!item) return;
      this.zoomAt(nextZoom || Math.max(this.zoom, 1.6), item.coords.x, item.coords.y, true);
      this.setSelected(item.id);
    }

    clampPan() {
      const rect = this.viewport.getBoundingClientRect();
      const width = 1600 * this.scale;
      const height = 1000 * this.scale;
      const padding = 80;
      if (width <= rect.width) {
        this.x = (rect.width - width) / 2;
      } else {
        this.x = Math.min(padding, Math.max(rect.width - width - padding, this.x));
      }
      if (height <= rect.height) {
        this.y = (rect.height - height) / 2;
      } else {
        this.y = Math.min(padding, Math.max(rect.height - height - padding, this.y));
      }
    }

    applyTransform() {
      this.world.style.transform = `translate3d(${this.x}px, ${this.y}px, 0) scale(${this.scale})`;
      this.world.style.setProperty('--marker-scale', 1 / this.scale);
    }

    emitZoom() {
      if (this.options.onZoom) this.options.onZoom(Math.round(this.zoom * 100));
    }

    escape(value) {
      return String(value).replace(/[&<>"']/g, (character) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#039;' }[character]));
    }

    destroy() {
      this.destroyed = true;
      this.resizeObserver.disconnect();
      this.viewport.removeEventListener('pointerdown', this.onPointerDown);
      this.viewport.removeEventListener('pointermove', this.onPointerMove);
      this.viewport.removeEventListener('pointerup', this.onPointerUp);
      this.viewport.removeEventListener('pointercancel', this.onPointerUp);
      this.viewport.removeEventListener('wheel', this.onWheel);
      this.viewport.removeEventListener('keydown', this.onKeyDown);
      this.world.remove();
    }
  }

  window.VLMap = VLMap;
})();
