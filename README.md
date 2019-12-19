Logro
===

## Introduction

Logro is a log rolling package with page cache control in Go. Inspired by [lumberjack](https://github.com/natefinch/lumberjack)

Logro is built for high performance:

- __Write with buffer__

    All log data will be written to a user-space buffer first, 
    then flush to the log file.
    
- __Sync in background__

    Use sync in background avoiding write stall in user-facing.
    
    ps: OS can do the page cache flush by itself, 
        but it may create a burst of write I/O when dirty pages hit a threshold.
    
- __Clean page cache__

    It's meaningless to keep log files' data in page cache,
    so when the dirty pages are too many or we need reopen a new file,
    Logro will sync data to disk, then drop the page cache.
    
- __...__
    
## Methods

- __WriteSyncCloser__

    Logro implements such methods:

    ```
        Write(p []byte) (written int, err error)
        Sync() (err error)
        Close() (err error)
    ```

    Could satisfy most of log packages.
    
## Rotation

e.g. The log file's name is ```a.log```, the log files will be:

```
    a.log
    a-time.log
    a-time.log
    a-time.log
    a-time.log
    ....
```

Log shippers such as ELK's filebeat can set path to:
    
```
    a.log    
```

### Control

Logro control rotation by file size only, it's simple and enough for the most cases.
(Now we usually use log shippers to collect logs to databases,
but not login machines and grep data)

### Warning
        
1. The date in backup file name maybe not correct all the time.
    (some log entries won't belong to this interval)    

2. It will clean up log files when they are too many (see Config.MaxBackups).
    
## Example

### Stdlib Logger

```
    r, _ := New(&conf)
    log.New(r, "", log.Ldate)
```

### Zap Logger

```
    r, _ := New(&conf)
    zapcore.AddSync(r)
    ...
```

## Acknowledgments

- [lumberjack](https://github.com/natefinch/lumberjack)
