import {
  Alert,
  Badge,
  Button,
  Card,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
  Checkbox,
  ClosableTag,
  Dialog,
  Input,
  NativeSelect,
  Select,
  Table,
  Tag,
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
export const nativeSelect = (
  // @ts-expect-error className would bypass tokens
  <NativeSelect label="a" options={[]} className="x" />
);
// @ts-expect-error label is mandatory
export const nativeSelectLabel = <NativeSelect options={[]} />;
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
export const alert = <Alert tone="info" className="x" />;
// @ts-expect-error style would bypass tokens
export const alertStyle = <Alert tone="info" style={{ color: "red" }} />;
export const tag = <Tag className="x" />;
// @ts-expect-error className would bypass tokens
export const closableTag = <ClosableTag closeLabel="a" className="x" />;
// @ts-expect-error className would bypass tokens
export const badge = <Badge className="x" />;
// @ts-expect-error unknown tone
export const badgeTone = <Badge tone="info" />;
