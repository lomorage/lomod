// Same extension -> mime map the old gallery.js/inbox.js used.
const EXT_TO_MIME = {
  jpg: 'image/jpeg',
  jpeg: 'image/jpeg',
  png: 'image/png',
  webp: 'image/webp',
  dng: 'image/DNG',
  tif: 'image/tiff',
  tiff: 'image/tiff',
  heif: 'image/heif',
  heic: 'image/heic',
  zip: 'livephoto/zip',
  '3gp': 'video/3gpp',
  '3g2': 'video/3gpp2',
  mov: 'video/mp4',
  mp4: 'video/mp4',
  avi: 'video/x-msvideo',
  mpg: 'video/mpeg',
  mpeg: 'video/mpeg',
  webm: 'video/webm',
};

export function getMimeType(filename) {
  const ext = filename.split('.').pop().toLowerCase();
  return EXT_TO_MIME[ext] || '';
}
