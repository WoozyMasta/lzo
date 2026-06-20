#include <errno.h>
#include <limits.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include <lzo/lzo1x.h>

static unsigned char *read_input(const char *path, lzo_uint *size) {
    FILE *file = fopen(path, "rb");
    if (file == NULL) {
        fprintf(stderr, "open %s: %s\n", path, strerror(errno));
        return NULL;
    }
    if (fseek(file, 0, SEEK_END) != 0) {
        fprintf(stderr, "seek %s: %s\n", path, strerror(errno));
        fclose(file);
        return NULL;
    }

    long length = ftell(file);
    if (length < 0 || (unsigned long)length > (unsigned long)LZO_UINT_MAX) {
        fprintf(stderr, "invalid input size: %ld\n", length);
        fclose(file);
        return NULL;
    }
    rewind(file);

    unsigned char *data = malloc(length > 0 ? (size_t)length : 1);
    if (data == NULL) {
        fprintf(stderr, "allocate input: %s\n", strerror(errno));
        fclose(file);
        return NULL;
    }
    if (length > 0 && fread(data, 1, (size_t)length, file) != (size_t)length) {
        fprintf(stderr, "read %s: %s\n", path, strerror(errno));
        free(data);
        fclose(file);
        return NULL;
    }

    fclose(file);
    *size = (lzo_uint)length;
    return data;
}

static int write_output(const unsigned char *data, lzo_uint size) {
    if (size > 0 && fwrite(data, 1, size, stdout) != size) {
        fprintf(stderr, "write output: %s\n", strerror(errno));
        return 1;
    }
    return 0;
}

static int compress_input(const unsigned char *input, lzo_uint input_size, int high) {
    lzo_uint output_size = input_size + input_size / 16 + 64 + 3;
    unsigned char *output = malloc(output_size > 0 ? output_size : 1);
    size_t work_size = high ? LZO1X_999_MEM_COMPRESS : LZO1X_1_MEM_COMPRESS;
    void *work = malloc(work_size);
    if (output == NULL || work == NULL) {
        fprintf(stderr, "allocate compression buffers: %s\n", strerror(errno));
        free(output);
        free(work);
        return 1;
    }

    int result = high
        ? lzo1x_999_compress(input, input_size, output, &output_size, work)
        : lzo1x_1_compress(input, input_size, output, &output_size, work);
    free(work);
    if (result != LZO_E_OK) {
        fprintf(stderr, "compress: liblzo2 error %d\n", result);
        free(output);
        return 1;
    }

    result = write_output(output, output_size);
    free(output);
    return result;
}

static int decompress_input(const unsigned char *input, lzo_uint input_size, lzo_uint output_size) {
    lzo_uint decoded_size = output_size;
    unsigned char *output = malloc(output_size > 0 ? output_size : 1);
    if (output == NULL) {
        fprintf(stderr, "allocate output: %s\n", strerror(errno));
        return 1;
    }

    int result = lzo1x_decompress_safe(input, input_size, output, &decoded_size, NULL);
    if (result != LZO_E_OK) {
        fprintf(stderr, "decompress: liblzo2 error %d\n", result);
        free(output);
        return 1;
    }

    result = write_output(output, decoded_size);
    free(output);
    return result;
}

int main(int argc, char **argv) {
    if (argc < 3 || argc > 4) {
        fprintf(stderr, "usage: %s <compress-fast|compress-high|decompress> <input> [output-size]\n", argv[0]);
        return 2;
    }
    if (lzo_init() != LZO_E_OK) {
        fprintf(stderr, "initialize liblzo2\n");
        return 1;
    }

    lzo_uint input_size = 0;
    unsigned char *input = read_input(argv[2], &input_size);
    if (input == NULL) {
        return 1;
    }

    int result;
    if (strcmp(argv[1], "compress-fast") == 0 && argc == 3) {
        result = compress_input(input, input_size, 0);
    } else if (strcmp(argv[1], "compress-high") == 0 && argc == 3) {
        result = compress_input(input, input_size, 1);
    } else if (strcmp(argv[1], "decompress") == 0 && argc == 4) {
        char *end = NULL;
        errno = 0;
        unsigned long output_size = strtoul(argv[3], &end, 10);
        if (errno != 0 || end == argv[3] || *end != '\0' || output_size > LZO_UINT_MAX) {
            fprintf(stderr, "invalid output size: %s\n", argv[3]);
            free(input);
            return 2;
        }
        result = decompress_input(input, input_size, (lzo_uint)output_size);
    } else {
        fprintf(stderr, "invalid operation\n");
        result = 2;
    }

    free(input);
    return result;
}
