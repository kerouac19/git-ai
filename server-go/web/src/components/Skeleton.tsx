interface Props {
  width?: string | number;
  height?: string | number;
  circle?: boolean;
  className?: string;
  style?: React.CSSProperties;
}

export default function Skeleton({ width, height, circle, className = "", style }: Props) {
  const combinedStyle: React.CSSProperties = {
    width: width ?? "100%",
    height: height ?? "1rem",
    borderRadius: circle ? "50%" : "4px",
    ...style,
  };

  return <div className={`skeleton ${className}`} style={combinedStyle} />;
}
