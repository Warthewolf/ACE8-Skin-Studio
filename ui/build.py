# Puts the window's page together: head.html (styles), body.html (layout), app.js (script with the
# start-up code at the end), slots.js (new skin slots), emblems.js (new emblems), pilots.js (helmets
# and portraits), pilotnames.js (names on the radio) and savetools.js (the game's save), all
# inserted before the start-up code.
import os
here = os.path.dirname(os.path.abspath(__file__))
rd = lambda n: open(os.path.join(here, n), encoding='utf-8').read()
app = rd('app.js')
marker = "// ---------------------------------------------------------------- start"
i = app.index(marker)
if 'function slotNotReady()' not in app:
    app = app[:i] + rd('slots.js') + "\n" + rd('emblems.js') + "\n" + rd('pilots.js') + "\n" + rd('pilotnames.js') + "\n" + rd('savetools.js') + "\n" + app[i:]
page = rd('head.html') + rd('body.html') + "\n" + app + "</body>\n</html>\n"
# the logo (emblem.png) and the window icon (favicon.png) go into the page itself
import base64
for key, name in (('{{EMBLEM}}', 'emblem.png'), ('{{FAVICON}}', 'favicon.png')):
    page = page.replace(key, 'data:image/png;base64,' + base64.b64encode(open(os.path.join(here, name), 'rb').read()).decode())
open(os.path.join(here, 'index.html'), 'w', encoding='utf-8').write(page)
print('index.html', len(rd('index.html')))
