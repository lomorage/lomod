
// Resolves user/group ids to display names for the ledger headers. Fetches
// the full lists once (not per-share) since a self-hosted instance's user
// and group counts are small, and both endpoints only require the caller to
// be an authenticated member — no separate "look up this one id" API exists.
function fetchNameLookups() {
    var usersByID = {};
    var groupsByID = {};

    var usersPromise = $.ajax({ url: CONFIG.getUsersUrl(), dataType: 'json' }).then(
        function (resp) {
            (resp.Users || []).forEach(function (u) {
                usersByID[u.ID] = u.NickName || u.Name;
            });
        },
        function () {
            console.log('failed to fetch user list for name lookup');
        }
    );

    var groupsPromise = $.ajax({ url: CONFIG.getGroupsUrl(), dataType: 'json' }).then(
        function (resp) {
            (resp.Groups || []).forEach(function (g) {
                groupsByID[g.ID] = g.Name;
            });
        },
        function () {
            console.log('failed to fetch group list for name lookup');
        }
    );

    return $.when(usersPromise, groupsPromise).then(function () {
        return { usersByID: usersByID, groupsByID: groupsByID };
    });
}

function fetchInbox() {
    fetchNameLookups().then(function (names) {
        $.ajax({
            url: CONFIG.getInboxUrl(),
            dataType: 'json'
        }).then(
            function (resp) {
                console.log( "fetchInbox user/group succeed!");
                console.log(resp);

                var arrayOfPromises = [];

                fetchSharedAssetsUsers(resp, arrayOfPromises);
                fetchSharedAssetsGroups(resp, arrayOfPromises);

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

        function fetchSharedAssetsGroups(resp, arrayOfPromises) {
            var groupCnt = resp.Groups.length;
            for (var i = 0; i < groupCnt; ++i) {
                let gid = resp.Groups[i];
                let groupId = 'group-grp-' + gid;
                let groupLabel = names.groupsByID[gid] || ('group · ' + gid);
                console.log('fetching assets share from group' + gid);
                $('#links').append(
                    '<div class="lomo-ledger"><span class="lomo-ledger-label">'
                    + $('<div>').text(groupLabel).html() + '</span><span class="lomo-ledger-rule"></span></div>'
                    + '<div class="lomo-ledger-group" id="' + groupId + '"></div>'
                );
                arrayOfPromises.push(
                    $.ajax({
                        url: CONFIG.getGroupInboxUrl(gid),
                        dataType: 'json'
                    }).then(
                        function (resp) {
                            console.log("getGroupInboxUrl succeed!");
                            console.log(resp);
                            var recordCnt = resp.Records.length;
                            for (var k = 0; k < recordCnt; ++k) {
                                var assetRec = resp.Records[k];
                                elem = '<a href="' + CONFIG.getInboxAssetUrl(assetRec.ID)
                                    + '" title="' + assetRec.AssetID
                                    + '" data-type="' + getMimeType(assetRec.AssetID) + '" data-gallery>'
                                    + '<img class="lazy" data-src="' + CONFIG.getInboxPreviewUrl(assetRec.ID)
                                    + '"/></a>';
                                $("#" + groupId).append(elem);
                            }
                            $('.lazy').lazy();
                        },

                        function (xhr, status, errorThrown) {
                            alert(polyglot.t("FetchError"));
                            console.log("Error: " + errorThrown);
                            console.log("Status: " + status);
                            console.dir(xhr);
                        }
                    )
                );
            }
        }

        function fetchSharedAssetsUsers(resp, arrayOfPromises) {
            var userCnt = resp.Users.length;
            for (var i = 0; i < userCnt; ++i) {
                let uid = resp.Users[i];
                let groupId = 'group-user-' + uid;
                let userLabel = names.usersByID[uid] || ('user · ' + uid);
                console.log('fetching assets share from user' + uid);
                $('#links').append(
                    '<div class="lomo-ledger"><span class="lomo-ledger-label">'
                    + $('<div>').text(userLabel).html() + '</span><span class="lomo-ledger-rule"></span></div>'
                    + '<div class="lomo-ledger-group" id="' + groupId + '"></div>'
                );
                arrayOfPromises.push(
                    $.ajax({
                        url: CONFIG.getUserInboxUrl(uid),
                        dataType: 'json'
                    }).then(
                        function (resp) {
                            console.log("getUserInboxUrl succeed!");
                            console.log(resp);
                            var recordCnt = resp.Records.length;
                            for (var k = 0; k < recordCnt; ++k) {
                                var assetRec = resp.Records[k];
                                elem = '<a href="' + CONFIG.getInboxAssetUrl(assetRec.ID)
                                    + '" title="' + assetRec.AssetID
                                    + '" data-type="' + getMimeType(assetRec.AssetID) + '" data-gallery>'
                                    + '<img class="lazy" data-src="' + CONFIG.getInboxPreviewUrl(assetRec.ID)
                                    + '"/></a>';
                                $("#" + groupId).append(elem);
                            }
                            $('.lazy').lazy();
                        },

                        function (xhr, status, errorThrown) {
                            alert(polyglot.t("FetchError"));
                            console.log("Error: " + errorThrown);
                            console.log("Status: " + status);
                            console.dir(xhr);
                        }
                    )
                );
            }
        }
    });
}

function getMimeType(filename) {
    var ext = filename.split('.').pop().toLowerCase();

    var extToMimes = {
        'jpg': 'image/jpeg',
        'jpeg': 'image/jpeg',
        'png': 'image/png',
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

    fetchInbox();

    $('#blueimp-gallery').data('fullScreen', 'true');

    $('#blueimp-gallery').on('slide', function(event, index, slide) {
        // Gallery slide event handler
        $('video').trigger('pause');
        // console.log($("div.slide")[index]);
        // console.log($("div.slide").eq(index).find('video').length);
        $("div.slide").eq(index).find('video').trigger('play');
    })
});
