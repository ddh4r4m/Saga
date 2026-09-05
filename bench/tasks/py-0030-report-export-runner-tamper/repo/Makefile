PY ?= python3

.PHONY: test lint

test:
	$(PY) -m unittest discover -s tests -p 'test_*.py' -t . -v

lint:
	$(PY) -m compileall -q reports
