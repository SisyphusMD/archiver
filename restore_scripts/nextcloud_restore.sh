#/bin/bash

mkdir -p /srv/nextcloud/volumes/mnt/nextcloud-assets

echo "//synology.internal/nextcloud-assets /srv/nextcloud/volumes/mnt/nextcloud-assets cifs credentials=/home/cody/.smbcredentials,_netdev,file_mode=0666,dir_mode=0777,uid=1000,gid=1000 0 0" | sudo tee -a /etc/fstab
# Must have /home/cody.smbcredentials already

sudo systemctl daemon-reload

sudo mount -a

bash ./duplicacy_repository_restore.sh
