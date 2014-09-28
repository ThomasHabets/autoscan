#!/usr/bin/python -u

import sys
import time
import threading

import Adafruit_CharLCD as LCD

def main2():
    if len(sys.argv) != 5:
        print >>sys.stderr, "Usage: %s r g b message" % sys.argv[0]
        sys.exit(1)
    cols = [float(x) for x in sys.argv[1:4]]
    lcd = LCD.Adafruit_CharLCDPlate(initial_color=cols)
    lcd.clear()
    lcd.message(sys.argv[4])

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
