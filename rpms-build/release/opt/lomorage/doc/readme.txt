# Readme

This is the home directory which stores each user's photos uploaded.

**Please DO NOT modify the folders/files directly, if you need to delete files, or organize photos with album/tags, do that with Lomorage APP, if there is anything you can't do with APP, feel free to reach us at support@lomorage.com**

The folder structure is as below:

## Photos/master/yyyy/mm/dd

- The original photos are organized in year/month/day, and the time is in UTC b/c user may have photos in different places with different timezone, the time will be converted to local timezone on Lomorage APP.

- Live Photos on iOS will stored as zip files, which has two files inside, one for video and one for image, and there is another image file has the same file name with the zip file, which is the same image file in the zip, so you don't need unzip the file to peek what's inside.

- Windows may not support the heic/heif image or hecv mov video uploaded from iOS, you need get the HEVC/HEIC entension on Windows to view it if you are on Windows.

## Photos/preview/yyyy/mm/dd

This folder stores the preview generated in different dimensions, and will be used by Lomorage APP to load the image quickly.

## Photos/lomodTrash

This folder stores the files you deleted from Lomorage APP, if you don't want to keep it anymore, you can delete those files in the folder, we keep it here just in case you delete photo unintentional.

## lomodTemp

This is the temporary folder used for storing the temporary files uploaded.

## assets.db

This is the database that store the users account, hashed/encrypted password, assets hash, metadata etc, all needed for Lomorage APP to work. 
