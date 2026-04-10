import * as p from "@clack/prompts";
import color from "picocolors";

export { p, color };

export function header(text: string) {
  p.intro(color.bgCyan(color.black(` ${text} `)));
}

export function success(text: string) {
  p.log.success(color.green(text));
}

export function warn(text: string) {
  p.log.warn(color.yellow(text));
}

export function error(text: string) {
  p.log.error(color.red(text));
}

export function info(text: string) {
  p.log.info(text);
}
