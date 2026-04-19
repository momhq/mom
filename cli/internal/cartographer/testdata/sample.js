// Sample JavaScript file for Cartographer AST extraction tests.

class DataProcessor {
  constructor(config) {
    this.config = config;
  }

  process(record) {
    return record;
  }
}

function loadConfig(path) {
  return { path };
}

const formatValue = (val) => String(val);

const helper = function namedHelper(x) {
  return x;
};

let counter = 0;
