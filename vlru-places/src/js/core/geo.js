/** География: центр города, расстояния, границы карты. Координаты везде в порядке [lng, lat], как в GeoJSON. */

/** Центральная площадь Владивостока — точка отсчёта «расстояния от центра». */
export const CITY_CENTER = [131.8855, 43.1155];

/** Стартовый вид карты по сценарию: город — центр, природа и море — шире. */
export const SCENARIO_VIEW = {
  city: { center: [131.905, 43.098], zoom: 11.6 },
  nature: { center: [131.95, 43.35], zoom: 7.4 },
  sea: { center: [131.92, 42.98], zoom: 8.6 },
  now: { center: [131.9, 43.11], zoom: 11.8 },
};

/** Ограничение панорамирования: Приморский край с запасом. */
export const MAX_BOUNDS = [
  [128.5, 41.2],
  [140.5, 48.8],
];

const toRad = (deg) => (deg * Math.PI) / 180;

/** Расстояние по прямой в километрах. */
export function haversineKm([lng1, lat1], [lng2, lat2]) {
  const R = 6371;
  const dLat = toRad(lat2 - lat1);
  const dLng = toRad(lng2 - lng1);
  const a = Math.sin(dLat / 2) ** 2 + Math.cos(toRad(lat1)) * Math.cos(toRad(lat2)) * Math.sin(dLng / 2) ** 2;
  return 2 * R * Math.asin(Math.sqrt(a));
}

export const distanceFromCenter = (coords) => haversineKm(CITY_CENTER, coords);

export function boundsOf(coordsList) {
  const lngs = coordsList.map((c) => c[0]);
  const lats = coordsList.map((c) => c[1]);
  return [
    [Math.min(...lngs), Math.min(...lats)],
    [Math.max(...lngs), Math.max(...lats)],
  ];
}
