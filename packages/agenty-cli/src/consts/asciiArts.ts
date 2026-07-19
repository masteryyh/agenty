export const BoxStyle = `
╔═╗╔═╗╔═╗╔╗╔╔╦╗╦ ╦
╠═╣║ ╦║╣ ║║║ ║ ╚╦╝
╩ ╩╚═╝╚═╝╝╚╝ ╩  ╩ `;

export const PixelStyle = `
▄▀█ █▀▀ █▀▀ █▄ █ ▀█▀ █▄█
█▀█ █▄█ ██▄ █ ▀█  █   █ `;

export const LightBoxStyle = `
┌─┐┌─┐┌─┐┌┐┌┌┬┐┬ ┬
├─┤│ ┬│┤ │││ │ └┬┘
┴ ┴└─┘└─┘┘└┘ ┴  ┴ `;

export const BoldStyle = `
┏━┓┏━┓┏━┓┏┓┏┏┳┓┳ ┳
┣━┫┃ ┳┃┫ ┃┃┃ ┃ ┗┳┛
┻ ┻┗━┛┗━┛┛┗┛ ┻  ┻ `;

export const RoundStyle = `
╭─╮╭─╮╭─╮╭╮╭╭┬╮╮ ╭
├─┤│ ┬│┤ │││ │ ╰┬╯
┴ ┴╰─╯╰─╯╯╰╯ ┴  ┴ `;

export const ASCIIArts = [
	BoxStyle,
	PixelStyle,
	LightBoxStyle,
	BoldStyle,
	RoundStyle,
];

let cursor = 0;
export function pickAsciiArt(): string {
	const idx = cursor % ASCIIArts.length;
	cursor += 1;
	return ASCIIArts[idx].replace(/^\n/, "");
}
