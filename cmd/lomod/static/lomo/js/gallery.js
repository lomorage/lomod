function fetchMonthLevelMerkleTree() {
    return $.ajax({
        url: CONFIG.getMonthLevelMerkleTreeUrl(),
        dataType: 'json'
    });
}

function fetchAssetLevelMerkleTree() {
    fetchMonthLevelMerkleTree().then(
        function (monthLevelTree) {
            console.log( "fetchMonthLevelMerkleTree succeed!");
            //console.log(monthLevelTree);

            var arrayOfPromises = [];
            var yearCnt = monthLevelTree.Years.length;
            for (var i = 0; i < yearCnt; ++i) {
                var yearRec = monthLevelTree.Years[yearCnt-i-1];
                let year = yearRec.Year;
                var monthCnt = yearRec.Months.length;
                for (var j = 0; j < monthCnt; ++j) {
                    var monthRec = yearRec.Months[monthCnt-j-1];
                    let month = monthRec.Month;
                    //console.log('fetching assets for ' + year + '/' + month);
                    let groupId = 'group-' + year + '-' + month;
                    $('#links').append(
                        '<div class="lomo-ledger"><span class="lomo-ledger-label">'
                        + year + ' &middot; ' + String(month).padStart(2, '0')
                        + '</span><span class="lomo-ledger-rule"></span></div>'
                        + '<div class="lomo-ledger-group" id="' + groupId + '"></div>'
                    );
                    arrayOfPromises.push(
                        $.ajax({
                            url: CONFIG.getAssetLevelMerkleTreeUrl(year, month),
                            dataType: 'json'
                        }).then(
                            function (assetLevelTree) {
                                //console.log("fetchAsset record succeed!");
                                //console.log(assetLevelTree)
                                var dayCnt = assetLevelTree.Days.length;
                                for (var k = 0; k < dayCnt; ++k) {
                                    var dayRec = assetLevelTree.Days[dayCnt-k-1];
                                    var assetCnt = dayRec.Assets.length;
                                    for (var m = 0; m < assetCnt; ++m) {
                                        var assetRec = dayRec.Assets[m];
                                        var mimeType = getMimeType(assetRec.Name);
                                        var mediaType = mimeType.split('/')[0]
                                        if (mediaType === "image") {
                                            elem = '<a href="' + CONFIG.getAssetUrl(assetRec.Name)
                                                + '" title="' + assetRec.Name
                                                + '" data-type="' + getMimeType(assetRec.Name) +'" data-gallery>'
                                                + '<div class="image-thumbnail"><img class="lazy" data-src="' + CONFIG.getPreviewUrl(assetRec.Name)
                                                + '"/></div></a>';
                                            $( "#" + groupId ).append(elem);
                                        } else {
                                            elem = '<a href="' + CONFIG.getAssetUrl(assetRec.Name)
                                                + '" title="' + assetRec.Name
                                                + '" data-type="' + getMimeType(assetRec.Name) +'" data-gallery>'
                                                + '<div class="video-thumbnail"><img class="lazy" data-src="' + CONFIG.getPreviewUrl(assetRec.Name)
                                                + '"/></div></a>';
                                            $( "#" + groupId ).append(elem);
                                        }
                                        //console.log("Name: " + assetRec.Name + ", Hash: " + assetRec.Hash);

                                    }
                                }
                                $('.lazy').lazy();
                            },

                            function( xhr, status, errorThrown ) {
                                alert( polyglot.t("FetchError") );
                                console.log( "Error: " + errorThrown );
                                console.log( "Status: " + status );
                                console.dir( xhr );
                            }
                        )
                    );
                }
            }

            $.when.apply($, arrayOfPromises).then(function() {
                console.log("all done!");
            });
        },

        function( xhr, status, errorThrown ) {
            alert( polyglot.t("FetchError") );
            console.log( "Error: " + errorThrown );
            console.log( "Status: " + status );
            console.dir( xhr );
        }
    )
}

function getMimeType(filename) {
    var ext = filename.split('.').pop().toLowerCase();

    var extToMimes = {
        'jpg': 'image/jpeg',
        'jpeg': 'image/jpeg',
        'png': 'image/png',
        'webp': 'image/webp',
        'dng': 'image/DNG',
        'tif': 'image/tiff',
        'tiff': 'image/tiff',
        'heif': 'image/heif',
        'heic': 'image/heic',
        'zip': 'livephoto/zip',
        '3gp': 'video/3gpp',
        '3g2': 'video/3gpp2',
        'mov': 'video/mp4',
        'mp4': 'video/mp4',
        'avi': 'video/x-msvideo',
        'mpg': 'video/mpeg',
        'mpeg': 'video/mpeg',
        'webm': 'video/webm',
    }

    if (extToMimes.hasOwnProperty(ext)) {
        return extToMimes[ext];
    }
    return '';
}

$(function() {
    $.ajaxSetup({
        headers: {
            "Authorization": "token=" + sessionStorage.getItem("token")
        }
    });

    if (sessionStorage.getItem("token") === null) {
        document.location.href = '/';
    }

    $('a#logout').text(polyglot.t("Logout") + sessionStorage.getItem("username"))
    $('a#logout').click(function() {
    sessionStorage.removeItem("token");
    sessionStorage.removeItem("userid");
    sessionStorage.removeItem("username");
    document.location.href = '/';
    });

    fetchAssetLevelMerkleTree();

    // $('#blueimp-gallery').data('fullScreen', 'true');

    $('#blueimp-gallery').on('slide', function(event, index, slide) {
        // Gallery slide event handler
        $('video').trigger('pause');
        // console.log($("div.slide")[index]);
        // console.log($("div.slide").eq(index).find('video').length);
        $("div.slide").eq(index).find('video').trigger('play');
    })

    $('#blueimp-gallery').on('deleted', function(event, index) {
        // Gallery deleted event handler
        console.log($("div.links")[index]);
        $("[data-gallery]")[index].remove();
        // console.log($("div.slide").eq(index).find('video').length);
    })
});
