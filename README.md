#### Identify duplicate photos

pdupe is a command line utility for identifying duplicate photos.  It works by building a color map summary of given files and comparing them for deviations.

The logic for building the color map was taken from the "Find duplicates" feature in Geeqie.

pdupe ignores exif data.  Since it analyzes the content of the photos, it's able to identify photos that have been altered slightly.
