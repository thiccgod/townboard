# townboard
![image](https://user-images.githubusercontent.com/91354035/137077526-90ff9ddd-89d7-4d15-aa86-36705667e813.png)
```
only tested on 1920x1080, default brightness/contrast
```
# build
```
clone opencv & opencv-contrib and build w/ mingw64
get tesseract-OCR and add to system path

git clone https://github.com/thiccgod/townboard.git

cd townboard

set CGO_CXXFLAGS=--std=c++11
export CGO_CPPFLAGS="-I%PATH_TO_OPENCV%\build\install\include"
export CGO_LDFLAGS="-L%PATH_TO_OPENCV%\build\install\x64\mingw\lib -llibopencv_core453 -llibopencv_face453 -llibopencv_videoio453 -llibopencv_imgproc453 -llibopencv_highgui453 -llibopencv_imgcodecs453 -llibopencv_objdetect453 -llibopencv_features2d453 -llibopencv_video453 -llibopencv_dnn453 -llibopencv_xfeatures2d453 -llibopencv_calib3d453 -llibopencv_photo453"

go build && ./townboard.exe
```

