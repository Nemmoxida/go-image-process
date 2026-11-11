1. the main is are: page to upload files (image file, ananotation text file, or a folder that contain 2 folder named image and annotation), page to pick augmentation method (a check box), and to download (the output from backend is a zip file)
2. for annotation here's how it works: annotation is an optionala file that user can upload if there is annotation then the image need to have the same exact amout and have the same naming fromat (the naming format is image_1.png/jpg and image_1.txt)
3. for folder the same check 2 folder inside first to check the name (need tobe image and annotation)
4. there is a legacy api in the api.md and readme. the legacy include a simple iamge processing one at a time (like rotate one image, or crop, or filter, etc) ignore it. the front end will focus on multiple augmentation, so just focus on the augmentation api. the example is on API.md
5. req body is a multipart form that contain: image, metadata(api body), and an optional annotation(txt file)
6. in the page where user can input the augmentation technique there are several technique that the user need to input the value like rotate, resize, flip, etc (refer to API.md)
