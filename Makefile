### Makefile --- 

## Author: shell@dsk
## Version: $Id: Makefile,v 0.0 2017/08/07 15:10:47 shell Exp $
## Keywords: 
## X-URL: 

all: clean build

build:
	go build github.com/shell909090/mannitol
	strip mannitol

clean:
	rm -f mannitol

### Makefile ends here
