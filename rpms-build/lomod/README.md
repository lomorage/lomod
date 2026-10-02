# Lomod
Lomod is a back-end service for personal video/photo backup, sharing and management. Check out https://lomorage.com/ for more detail about the product.

**Note:** This repository only holds binary releases for Linux with below architectures: 
- armhf: for raspberry pi arm32 user
- arm64: for raspberry pi arm64 user
- amd64: non-pi user

If you want to install on other OS, please refer https://lomorage.com/installation/

## Command Usage
```
GLOBAL OPTIONS:
   --backup-dir value          backup directory
   --backup-time value         daily local backup time. Format is like hh:mm:ss. Timezone is local running machine timezone (default: "02:00:00")
   --base value, -b value      base directory to store db file (default: "/home/qiwa/go/src/bitbucket.org/lomoware/lomo-backend")
   --check-interval value      interval to run consistent check. Unit in day (default: 7)
   --log-dir value             logfile directory
   --mdns-domain value         mdns search domain name (default: "local.")
   --mdns-name value           mdns service name (default: "qiwa-Aspire-ME600")
   --mdns-service value        mdns service type (default: "_lomod._tcp")
   --mount-dir value           mount directory to find out mounted usb disk (default: "/media")
   --no-mdns                   disable mdns for service discovery
   --no-mount                  this mode is mainly used at desktop version. Mount directory is what user defined when it starts lomod server, and no mount disk in this case
   --port value, -p value      (default: 8000)
   --preview-size value        list of image preview size. Multiple resolution is supported, and each is separated with ';'. Format is like <width1>x<height1>;<width2>x<height2>;... (default: "75x0;320x0")
   --preview-size-video value  list of video preview size. Multiple resolution is supported, and each is separated with ';'. Format is like <width1>x<height1>;<width2>x<height2>;... (default: "480x0")
   --help, -h                  show help
   --version, -v               print the version
   ```
### Mount-dir
Mount-dir argument is to specify the path for storing the photo / video assets. By default it is `/media` at raspberry pi, all external USB disks should also be mounted under this directory, so that `lomod` can read the list of USB disks and return to mobile client. User can also select the desired disks when user is created. The user's home path is `mount-dir + <usb disk UUID> + <user name>`.

If user doesn't use external USB disk, he needs add `--no-mount` option, thus, lomod will use `mount-dir + <user name>` as user's home path.
