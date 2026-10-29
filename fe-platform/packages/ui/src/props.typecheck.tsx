import { Button, Checkbox, Dialog, Input, Select, Table } from "./index";

// @ts-expect-error className would bypass tokens
export const button = <Button className="x" />;
// @ts-expect-error style would bypass tokens
export const buttonStyle = <Button style={{ color: "red" }} />;
// @ts-expect-error className would bypass tokens
export const input = <Input label="a" className="x" />;
// @ts-expect-error className would bypass tokens
export const checkbox = <Checkbox label="a" className="x" />;
// @ts-expect-error className would bypass tokens
export const select = <Select label="a" options={[]} className="x" />;
const dialogBase = { triggerLabel: "a", title: "a", description: "a" };
// @ts-expect-error className would bypass tokens
export const dialog = <Dialog {...dialogBase} className="x" />;
// @ts-expect-error className would bypass tokens
export const table = <Table caption="a" columns={[]} rows={[]} className="x" />;
