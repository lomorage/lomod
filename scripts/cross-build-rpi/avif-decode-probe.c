/* Does the libheif we are about to link libvips against actually decode AVIF?
 *
 * Asking the version number is not enough: AVIF decoding needs both a libheif
 * new enough to know the AV1 compression format (1.7.0+) and an AV1 decoder
 * plugin compiled into it (aom or dav1d), and distributions differ on whether
 * they enable one. Debian buster's libheif 1.3.2 has neither, which is why a
 * .avif reaches the Pi and gets stored but never gets a preview.
 *
 * Exits 0 when AVIF can be decoded. Anything else -- including failing to
 * compile at all, which is what happens on a libheif too old to have these
 * two symbols -- means the caller should build libheif from source instead.
 */
#include <libheif/heif.h>
#include <stdio.h>

int main(void)
{
	if (!heif_have_decoder_for_format(heif_compression_AV1)) {
		printf("libheif %s: no AV1 decoder\n", heif_get_version());
		return 1;
	}
	printf("libheif %s: AV1 decoder present\n", heif_get_version());
	return 0;
}
