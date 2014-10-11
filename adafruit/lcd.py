#!/usr/bin/python -u
#
# Copyright(c) Thomas Habets <thomas@habets.se> 2014
#
# I couldn't find an SPI interface written in Go, so I have to shell
# out to this.
#
# When a button is pressed, it writes the buttons name to stdout.
# Messages can be written to stdin in this format:
#   r|g|b|Row1|Row2
# Where (r,g,b) are backlight colors, and Row1 and Row2 are what's to
# be written on the two rows.
# E.g.:
#   1|0|0|Error:|Something worng!
#
import sys
import time
import threading

import Adafruit_CharLCD as LCD

def _ButtonThread(lcd):
    bs = [(LCD.SELECT, 'SELECT'),
          (LCD.UP, 'UP'),
          (LCD.DOWN, 'DOWN'),
          (LCD.LEFT, 'LEFT'),
          (LCD.RIGHT, 'RIGHT')]
    while True:
        for b, n in bs:
            if lcd.is_pressed(b):
                print n
        time.sleep(0.1)

def main():
    lcd = LCD.Adafruit_CharLCDPlate()
    bt = threading.Thread(target=_ButtonThread, args=(lcd,))
    bt.daemon = True
    bt.start()
    lcd = LCD.Adafruit_CharLCDPlate()
    lcd.clear()
    lcd.set_color(0, 0, 1)
    lcd.message('Starting up...')
    while True:
        line = sys.stdin.readline().strip()
        r,g,b,x,y = line.split('|')
        lcd.clear()
        lcd.set_color(float(r),float(g),float(b))
        lcd.message('%s\n%s' % (x, y))

if __name__ == '__main__':
    main()
