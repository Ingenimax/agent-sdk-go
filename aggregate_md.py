#!/usr/bin/env python3

import os
import re
import argparse
from pathlib import Path


def get_heading_level(content):
    """Determine the heading level of the first heading in the content."""
    heading_match = re.search(r'^(#+)\s+', content, re.MULTILINE)
    if heading_match:
        return len(heading_match.group(1))
    return 0


def adjust_headings(content, level_increase):
    """Adjust all headings in the content by increasing their level."""
    if level_increase <= 0:
        return content

    def increase_heading(match):
        return '#' * (len(match.group(1)) + level_increase) + match.group(2)

    return re.sub(r'^(#+)(\s+.+)$', increase_heading, content, flags=re.MULTILINE)


def aggregate_markdown_files(directory, output_file, exclude=None):
    """
    Aggregate all markdown files in a directory and its subdirectories into a single file.

    Args:
        directory: Path to the directory containing markdown files
        output_file: Path to the output file
        exclude: List of directories or files to exclude
    """
    if exclude is None:
        exclude = []

    # Convert to absolute paths for easier comparison
    directory = os.path.abspath(directory)
    exclude = [os.path.abspath(os.path.join(directory, path)) for path in exclude]

    # Find all markdown files
    md_files = []
    for root, dirs, files in os.walk(directory):
        # Skip excluded directories
        dirs[:] = [d for d in dirs if os.path.abspath(os.path.join(root, d)) not in exclude]

        for file in files:
            if file.lower().endswith('.md'):
                file_path = os.path.join(root, file)
                if file_path not in exclude and os.path.abspath(file_path) != os.path.abspath(output_file):
                    md_files.append(file_path)

    # Sort files by path for consistent output
    md_files.sort()

    # Create sections based on directory structure
    sections = {}
    for file_path in md_files:
        rel_path = os.path.relpath(file_path, directory)
        dir_path = os.path.dirname(rel_path)

        if dir_path not in sections:
            sections[dir_path] = []

        sections[dir_path].append(file_path)

    # Write the aggregated content
    with open(output_file, 'w', encoding='utf-8') as outfile:
        outfile.write(f"# Aggregated Markdown Documentation\n\n")
        outfile.write(f"*Generated on: {os.path.basename(output_file)}*\n\n")

        # Sort sections
        for section_path in sorted(sections.keys()):
            if section_path == '.':
                outfile.write(f"## Root Files\n\n")
            else:
                section_title = section_path.replace('/', ' > ')
                outfile.write(f"## {section_title}\n\n")

            # Process files in this section
            for file_path in sections[section_path]:
                file_name = os.path.basename(file_path)
                rel_path = os.path.relpath(file_path, directory)

                outfile.write(f"### File: {file_name}\n\n")
                outfile.write(f"*Path: {rel_path}*\n\n")

                try:
                    with open(file_path, 'r', encoding='utf-8') as infile:
                        content = infile.read()

                        # Adjust heading levels to fit in the hierarchy
                        content = adjust_headings(content, 3)

                        outfile.write(content)
                        outfile.write("\n\n---\n\n")
                except Exception as e:
                    outfile.write(f"**Error reading file: {str(e)}**\n\n---\n\n")

    print(f"Aggregated {len(md_files)} markdown files into {output_file}")
    print(f"Files were organized into {len(sections)} sections")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Aggregate markdown files into a single document")
    parser.add_argument("-d", "--directory", default=".", help="Directory to search for markdown files")
    parser.add_argument("-o", "--output", default="aggregated_markdown.md", help="Output file path")
    parser.add_argument("-e", "--exclude", nargs='+', default=[], help="Directories or files to exclude")

    args = parser.parse_args()

    aggregate_markdown_files(args.directory, args.output, args.exclude)
