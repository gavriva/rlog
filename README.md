## Features
1. Log file rotation.
1. Asynchronous output to file and/or to STDOUT.

### Runtime structure:
```
+------------------------+
|io.Writer               |
|from std logger         |
|{msg, info, time.Now()} |
+----------+-------------+
           |                       +-----------+        +----------------+
           +-------------------->  | Buffered  |        | file rotation  |
                                   | Channel   | +----> | msg format     |
           +-------------------->  |           |        |                |
           |                       +-----------+        +-------+--------+
+----------+-------------+                                      |
|api functions           |                                      |
|{msg, LEVEL, time.Now()}|                                      v
+------------------------+                              +----------------+        +------------+
                                                        | Buffered       |        |            |
                                                        | Channel        +------> |   STDOUT   |
                                                        |                |        |            |
                                                        +----------------+        +------------+
```
