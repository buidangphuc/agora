import {
  Breadcrumb,
  Button,
  Card,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
  Checkbox,
  Dialog,
  Input,
  Pagination,
  Select,
  Stepper,
  Table,
  Tabs,
  Timeline,
} from "./index";

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
// @ts-expect-error className would bypass tokens
export const card = <Card className="x" />;
// @ts-expect-error style would bypass tokens
export const cardStyle = <Card style={{ color: "red" }} />;
// @ts-expect-error className would bypass tokens
export const cardHeader = <CardHeader className="x" />;
// @ts-expect-error className would bypass tokens
export const cardTitle = <CardTitle className="x" />;
// @ts-expect-error className would bypass tokens
export const cardContent = <CardContent className="x" />;
// @ts-expect-error className would bypass tokens
export const cardFooter = <CardFooter className="x" />;
// @ts-expect-error unknown variant
export const buttonVariant = <Button variant="outline" />;
// @ts-expect-error className would bypass tokens
export const tabs = <Tabs items={[]} className="x" />;
export const pagination = (
  // @ts-expect-error className would bypass tokens
  <Pagination current={1} total={1} hrefFor={String} className="x" />
);
// @ts-expect-error className would bypass tokens
export const breadcrumb = <Breadcrumb items={[]} className="x" />;
// @ts-expect-error className would bypass tokens
export const stepper = <Stepper steps={[]} className="x" />;
// @ts-expect-error className would bypass tokens
export const timeline = <Timeline items={[]} className="x" />;
