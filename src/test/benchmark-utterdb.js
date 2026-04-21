import http from "k6/http";

export const options = {
  vus: 1000,
  duration: "30s",
};

export default function () {
  const i = Math.floor(Math.random() * 1000000);

  const url =
    `http://localhost:8080/write` +
    `?key=hi${i}` +
    `&value=val${i}` +
    `&keyType=string` +
    `&valueType=string`;

  http.post(url, null, {
    tags: { name: "write" },
  });
}
