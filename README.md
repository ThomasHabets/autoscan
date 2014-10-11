Autoscan

Copyright(c) Thomas Habets <thomas@habets.se> 2014

Scans from USB scanner directly into Google drive.


Dependencies
============
apt-get install sane-utils libsane-hpaio imagemagick


Install
=======

1) Install binary
-----------------
go install github.com/ThomasHabets/autoscan

2) Give access to USB storage to the user, say a dedicated "scanner" user
-------------------------------------------------------------------------
Put this in /dev/udev/rules.d/99-autoscan.rules:
  ATTRS{idVendor}=="04c5", ATTRS{idProduct}=="1155", SYMLINK+="usbscanner", GROUP="scanner"

Make sure whatever user you run as is member of "scanner" group.

3) Set up directory
------------
```
sudo mkdir -p /opt/autoscan/{bin,log,etc}
sudo chown -R scanner /opt/autoscan
cp autoscan extra/lcd.py /opt/autoscan/bin/
cp -ax web/{templates,static} /opt/autoscan/
```

4) Configure
------------
```
/opt/autoscan/bin -config=/opt/autoscan/etc/autoscan.conf -configure
```

5a) If you have an Adafruit 16x2 display
---------------------------------------
```
sudo apt-get install build-essential python-dev python-smbus python-pip git
sudo pip install RPi.GPIO
git clone https://github.com/adafruit/Adafruit_Python_CharLCD.git
cd Adafruit_Python_CharLCD
sudo python setup.py install
```

5b) Instead if you wired up buttons and LEDs, this is an example GPIO layout
----------------------------------------------------------------------------
  * ButtonSingle   22
  * ButtonDuplex   23
  * Button3        17
  * Button4        27
  * LED 1:         6 / 25
  * LED 2:         5 / 24

6) Create a wrapper script for ```scanimage```
----------------------------------------------
Such as:
```
#!/bin/sh
exec scanimage \
    -d fujitsu \
    -y 300 -x 300 \
    --page-width 300 \
    --page-height 300 \
    "$@"
```

7) Either use the start script in extra/, or start my some other means
----------------------------------------------------------------------
```
/opt/autoscan/bin/autoscan \
    -scanimage=/opt/autoscan/bin/scanimage-wrap \
    -templates=/opt/autoscan/templates \
    -static=/opt/autoscan/static \
    -socket=/opt/autoscan/run/autoscan.sock \
    -config=/opt/autoscan/etc/autoscan.conf \
    -logfile=/opt/autoscan/log/autoscan.log \
    -use_adafruit
```


Random notes
============
Bugs in gpio on github:
* not checking for EINTR
* race condition opening for output: direction not set before value sent.
* improper locking
